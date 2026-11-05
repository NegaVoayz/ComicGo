//go:build windows

package main

import (
	"log"
	"runtime"
	"sync/atomic"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	cfHDROP           = 15
	dvAspectContent   = 1
	tymedHGLOBAL      = 1
	droppedEffectCopy = 1
	sOK               = 0
	eNoInterface      = 0x80004002
	ePointer          = 0x80004003
	dvEFormatEtc      = 0x80040064
)

type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

var (
	iidUnknown    = guid{Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidDropTarget = guid{Data1: 0x00000122, Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}

	ole32                = windows.NewLazySystemDLL("ole32.dll")
	shell32              = windows.NewLazySystemDLL("shell32.dll")
	procOleInitialize    = ole32.NewProc("OleInitialize")
	procOleUninitialize  = ole32.NewProc("OleUninitialize")
	procRegisterDragDrop = ole32.NewProc("RegisterDragDrop")
	procRevokeDragDrop   = ole32.NewProc("RevokeDragDrop")
	procReleaseStgMedium = ole32.NewProc("ReleaseStgMedium")
	procDragQueryFile    = shell32.NewProc("DragQueryFileW")
	procDragFinish       = shell32.NewProc("DragFinish")
)

type formatEtc struct {
	Format       uint16
	_            uint16
	TargetDevice uintptr
	Aspect       uint32
	Index        int32
	Tymed        uint32
}

type stgMedium struct {
	Tymed     uint32
	_         uint32
	Handle    uintptr
	Interface uintptr
}

type dropTargetVtbl struct {
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr
	DragEnter      uintptr
	DragOver       uintptr
	DragLeave      uintptr
	Drop           uintptr
}

type winDropTarget struct {
	vtbl   *dropTargetVtbl
	refs   int32
	over   int32
	ui     *appUI
	pinner runtime.Pinner
}

var winDropVtbl = dropTargetVtbl{
	QueryInterface: windows.NewCallback(dropQueryInterface),
	AddRef:         windows.NewCallback(dropAddRef),
	Release:        windows.NewCallback(dropRelease),
	DragEnter:      windows.NewCallback(dropDragEnter),
	DragOver:       windows.NewCallback(dropDragOver),
	DragLeave:      windows.NewCallback(dropDragLeave),
	Drop:           windows.NewCallback(dropFiles),
}

func callMethod(proc, object uintptr, args ...uintptr) uintptr {
	values := make([]uintptr, 0, len(args)+1)
	values = append(values, object)
	values = append(values, args...)
	result, _, _ := syscall.SyscallN(proc, values...)
	return result
}

func pointerFromUintptr(v uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&v))
}

func uintptrFromPointer(p unsafe.Pointer) uintptr {
	return *(*uintptr)(unsafe.Pointer(&p))
}

func dropTargetFrom(this uintptr) *winDropTarget {
	if this == 0 {
		return nil
	}
	return (*winDropTarget)(pointerFromUintptr(this))
}

//go:nocheckptr
func dropQueryInterface(this, riid, ppv uintptr) uintptr {
	target := dropTargetFrom(this)
	if target == nil || ppv == 0 {
		return ePointer
	}
	id := (*guid)(pointerFromUintptr(riid))
	if *id != iidUnknown && *id != iidDropTarget {
		return eNoInterface
	}
	*(*uintptr)(pointerFromUintptr(ppv)) = this
	atomic.AddInt32(&target.refs, 1)
	return sOK
}

func dropAddRef(this uintptr) uintptr {
	if target := dropTargetFrom(this); target != nil {
		atomic.AddInt32(&target.refs, 1)
	}
	return 1
}

func dropRelease(this uintptr) uintptr {
	target := dropTargetFrom(this)
	if target == nil {
		return 0
	}
	return uintptr(atomic.AddInt32(&target.refs, -1))
}

//go:nocheckptr
func hasFileDrop(dataObject uintptr) bool {
	if dataObject == 0 {
		return false
	}
	format := formatEtc{Format: cfHDROP, Aspect: dvAspectContent, Tymed: tymedHGLOBAL}
	method := *(*uintptr)(unsafe.Add(pointerFromUintptr(dataObject), 5*uintptrSize))
	return callMethod(method, dataObject, uintptrFromPointer(unsafe.Pointer(&format))) == sOK
}

func setDropState(target *winDropTarget, over bool) {
	if target != nil && target.ui != nil {
		value := int32(0)
		if over {
			value = 1
		}
		if atomic.SwapInt32(&target.over, value) != value {
			target.ui.post(dropStateEvent{over: over})
		}
	}
}

//go:nocheckptr
func dropDragEnter(this, dataObject, keyState, point, effect uintptr) uintptr {
	target := dropTargetFrom(this)
	if !hasFileDrop(dataObject) {
		if effect != 0 {
			*(*uint32)(pointerFromUintptr(effect)) = 0
		}
		return dvEFormatEtc
	}
	setDropState(target, true)
	if effect != 0 {
		*(*uint32)(pointerFromUintptr(effect)) = droppedEffectCopy
	}
	return sOK
}

//go:nocheckptr
func dropDragOver(this, dataObject, keyState, point, effect uintptr) uintptr {
	if !hasFileDrop(dataObject) {
		if effect != 0 {
			*(*uint32)(pointerFromUintptr(effect)) = 0
		}
		return dvEFormatEtc
	}
	setDropState(dropTargetFrom(this), true)
	if effect != 0 {
		*(*uint32)(pointerFromUintptr(effect)) = droppedEffectCopy
	}
	return sOK
}

func dropDragLeave(this uintptr) uintptr {
	setDropState(dropTargetFrom(this), false)
	return sOK
}

//go:nocheckptr
func dropFiles(this, dataObject, keyState, point, effect uintptr) uintptr {
	target := dropTargetFrom(this)
	if effect != 0 {
		*(*uint32)(pointerFromUintptr(effect)) = 0
	}
	if !hasFileDrop(dataObject) {
		return dvEFormatEtc
	}
	format := formatEtc{Format: cfHDROP, Aspect: dvAspectContent, Tymed: tymedHGLOBAL}
	medium := stgMedium{}
	method := *(*uintptr)(unsafe.Add(pointerFromUintptr(dataObject), 3*uintptrSize))
	if callMethod(method, dataObject, uintptrFromPointer(unsafe.Pointer(&format)), uintptrFromPointer(unsafe.Pointer(&medium))) != sOK {
		return dvEFormatEtc
	}
	defer procReleaseStgMedium.Call(uintptrFromPointer(unsafe.Pointer(&medium)))
	paths := queryDroppedFiles(medium.Handle)
	if target != nil && target.ui != nil {
		target.ui.post(dropStateEvent{over: false})
		if len(paths) > 0 {
			target.ui.post(filesDroppedEvent{paths: paths})
		}
	}
	if effect != 0 {
		*(*uint32)(pointerFromUintptr(effect)) = droppedEffectCopy
	}
	return sOK
}

func queryDroppedFiles(hdrop uintptr) []string {
	if hdrop == 0 {
		return nil
	}
	count, _, _ := procDragQueryFile.Call(hdrop, 0xffffffff, 0, 0)
	paths := make([]string, 0, int(count))
	for i := uintptr(0); i < count; i++ {
		n, _, _ := procDragQueryFile.Call(hdrop, i, 0, 0)
		if n == 0 {
			continue
		}
		buf := make([]uint16, n+1)
		procDragQueryFile.Call(hdrop, i, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		paths = append(paths, windows.UTF16ToString(buf))
	}
	procDragFinish.Call(hdrop)
	return paths
}

const uintptrSize = unsafe.Sizeof(uintptr(0))

func (u *appUI) attachDrop(hwnd uintptr) {
	if hwnd == 0 || u.dropTarget != nil {
		return
	}
	hr, _, _ := procOleInitialize.Call(0)
	if int32(hr) < 0 {
		log.Printf("initialize OLE for file drop: %#x", hr)
		return
	}
	u.oleInitialized = true
	target := &winDropTarget{vtbl: &winDropVtbl, refs: 1, ui: u}
	target.pinner.Pin(target)
	u.dropTarget = target
	u.dropHWND = hwnd
	hr, _, _ = procRegisterDragDrop.Call(hwnd, uintptr(unsafe.Pointer(target)))
	if int32(hr) < 0 {
		log.Printf("RegisterDragDrop: %#x", hr)
		u.detachDrop()
	}
}

func (u *appUI) detachDrop() {
	target, ok := u.dropTarget.(*winDropTarget)
	if !ok || target == nil {
		return
	}
	u.dropTarget = nil
	if u.dropHWND != 0 {
		procRevokeDragDrop.Call(u.dropHWND)
		u.dropHWND = 0
	}
	target.pinner.Unpin()
	if u.oleInitialized {
		procOleUninitialize.Call()
		u.oleInitialized = false
	}
}
