<script lang="ts">
  import { onMount } from 'svelte'
  import {
    countComics,
    getAbout,
    getLayout,
    getSettings,
    importArchiveFile,
    importImageFolder,
    importDropped,
    listRules,
    on,
    pickLibrary,
    reloadLayout,
    removeLibrary,
    startScan,
    cancelScan,
  } from './lib/api'
  import type { About, ComicCard, ImportOutcome, ImportProgress, LayoutDoc, RuleInfo, Settings } from './lib/api'
  import { OnFileDrop } from '../wailsjs/runtime/runtime'
  import TopBar from './components/TopBar.svelte'
  import Welcome from './components/Welcome.svelte'
  import Section from './components/Section.svelte'
  import CardSlot from './components/CardSlot.svelte'
  import Reader from './components/Reader.svelte'
  import ComicInfo from './components/ComicInfo.svelte'
  import Toasts from './components/Toasts.svelte'
  import { error, ok } from './lib/toast'
  import * as Go from '../wailsjs/go/main/App'

  const logFront = (m: string) => {
    Go.ReportFrontend(m).catch(() => {})
  }

  interface ComicRef {
    id: number
    title: string
    pageCount: number
    resume: number
  }

  let settings: Settings | null = $state(null)
  let layout: LayoutDoc | null = $state(null)
  let filterRules: RuleInfo[] = $state([])
  let sortRules: RuleInfo[] = $state([])
  let scanning = $state(false)
  let scanProgress = $state<{ done: number; total: number } | null>(null)
  let refreshKey = $state(0)
  let indexed = $state(0)
  let reader: ComicRef | null = $state(null)
  let info: { id: number; title: string } | null = $state(null)
  let showAbout = $state(false)
  let about: About | null = $state(null)
  let searchText = $state('')
  let tagFilters = $state<string[]>([]) // exact tags ANDed into every section's query
  let winH = $state(0) // tracks window height so the grid can fill it (no dead space below)
  let importing = $state(false) // a conversion is running (file/folder picker or drop)
  let importProgress = $state<ImportProgress | null>(null) // last import:progress event
  let dragOver = $state(false) // an OS file drag is over the window (drop highlight)
  let dragDepth = 0

  function bump() {
    refreshKey += 1
  }

  // keep the top-bar "N indexed" readout in sync with backend data changes
  $effect(() => {
    const k = refreshKey
    void k
    if (settings && (settings.libraryRoots ?? []).length > 0) {
      countComics('')
        .then((n) => (indexed = n))
        .catch(() => {})
    }
  })

  async function loadAll() {
    try {
      const [s, l, rules] = await Promise.all([getSettings(), getLayout(), listRules()])
      settings = s
      layout = l
      filterRules = rules.filter((r) => r.kind === 'filter')
      sortRules = rules.filter((r) => r.kind === 'sort')
    } catch {
      /* backend not ready */
    }
    // A restart with saved roots should repopulate the shelf even if nothing
    // was scanned in this run yet (incremental: unchanged archives are skipped).
    if ((settings?.libraryRoots ?? []).length > 0) {
      startScan().catch(() => {})
    }
  }

  onMount(() => {
    loadAll()
    const un1 = on('scan:started', () => (scanning = true))
    const un2 = on('scan:progress', (p: any) => {
      scanProgress = { done: p.done ?? 0, total: p.total ?? 0 }
    })
    const un3 = on('scan:done', () => {
      scanning = false
      scanProgress = null
      bump()
      getLayout().then((l) => (layout = l)).catch(() => {})
    })
    const un4 = on('layout:changed', () => {
      getLayout().then((l) => (layout = l)).catch(() => {})
    })
    const un5 = on('scan:error', (e: any) => {
      error(e?.path ? `Scan: ${e.path}: ${e.error}` : `Scan error: ${e?.error ?? ''}`)
    })
    const un6 = on('import:progress', (p: any) => {
      importProgress = p
    })
    // Native file-drop handler: converts dropped files/folders into the library.
    // useDropTarget=false means a drop anywhere on the window imports.
    OnFileDrop((_x, _y, paths) => {
      if (paths && paths.length > 0) doImportDrop(paths)
    }, false)
    // A passive highlight so the user knows a drop will import. The drop itself
    // is intercepted by Wails' OnFileDrop (always preventDefaults file drops),
    // so these listeners only track whether to show the overlay.
    window.addEventListener('dragenter', onDragEnter)
    window.addEventListener('dragover', onDragOver)
    window.addEventListener('dragleave', onDragLeave)
    window.addEventListener('drop', onDrop)
    const onErr = (e: any) => {
      const st = e?.error?.stack ? String(e.error.stack).split('\n').slice(0, 8).join(' < ') : ''
      logFront('window.error: ' + (e?.message ?? '') + ' STACK[' + st + ']')
    }
    const onRej = (e: any) =>
      logFront('unhandledrejection: ' + (e?.reason?.message ?? String(e?.reason ?? '')))
    window.addEventListener('error', onErr)
    window.addEventListener('unhandledrejection', onRej)
    const onResize = () => (winH = window.innerHeight)
    onResize()
    window.addEventListener('resize', onResize)
    return () => {
      un1()
      un2()
      un3()
      un4()
      un5()
      un6()
      window.removeEventListener('error', onErr)
      window.removeEventListener('unhandledrejection', onRej)
      window.removeEventListener('resize', onResize)
      window.removeEventListener('dragenter', onDragEnter)
      window.removeEventListener('dragover', onDragOver)
      window.removeEventListener('dragleave', onDragLeave)
      window.removeEventListener('drop', onDrop)
    }
  })

  async function addFolder() {
    try {
      const s = await pickLibrary()
      if (s) settings = s
      bump()
    } catch (e: any) {
      error(e?.message ?? 'Could not add folder')
    }
  }
  function doScan() {
    startScan().catch((e: any) => error(e?.message ?? 'Could not start scan'))
  }
  function doCancel() {
    cancelScan()
  }
  async function doReloadLayout() {
    try {
      await reloadLayout()
      bump() // force section counts to re-query too
    } catch (e: any) {
      error(e?.message ?? 'Could not reload layout.json')
    }
  }

  // ---- import ----

  function nameOf(path: string): string {
    const trimmed = path.replace(/[\\/]+$/, '')
    const parts = trimmed.split(/[\\/]/)
    return parts[parts.length - 1] || path
  }

  const importLabel = $derived(
    importProgress
      ? importProgress.total > 0
        ? `Importing ${importProgress.name}… ${importProgress.done}/${importProgress.total} pages`
        : `Importing ${importProgress.name}… page ${importProgress.done}`
      : importing
        ? 'Importing…'
        : ''
  )

  // One toast per finished comic; a cancelled dialog resolves empty and stays quiet.
  function toastOutcomes(outs: ImportOutcome[]) {
    if (!outs || outs.length === 0) return
    if (outs.length === 1) {
      const o = outs[0]
      ok(`Imported ${nameOf(o.path)} — ${o.pageCount} pages`)
    } else {
      ok(`Imported ${outs.length} comics into the library`)
    }
    bump()
  }

  async function runImport(work: Promise<ImportOutcome[]>) {
    if (importing) return
    importing = true
    importProgress = null
    try {
      toastOutcomes(await work)
    } catch (e: any) {
      error(e?.message ?? 'Import failed')
    } finally {
      importing = false
      importProgress = null
    }
  }

  function doImportFile() {
    // Resolve to an empty array when the dialog was cancelled (empty path).
    runImport(importArchiveFile().then((o) => (o && o.path ? [o] : [])))
  }
  function doImportFolder() {
    runImport(importImageFolder().then((o) => (o && o.path ? [o] : [])))
  }
  function doImportDrop(paths: string[]) {
    runImport(importDropped(paths))
  }

  // ---- drag-and-drop highlight (import happens via the native Wails drop) ----

  function hasFiles(e: DragEvent): boolean {
    return !!e.dataTransfer && Array.from(e.dataTransfer.types ?? []).includes('Files')
  }
  function onDragEnter(e: DragEvent) {
    if (!hasFiles(e)) return
    e.preventDefault()
    dragDepth++
    dragOver = true
  }
  function onDragOver(e: DragEvent) {
    if (!hasFiles(e)) return
    e.preventDefault()
    if (e.dataTransfer) e.dataTransfer.dropEffect = 'copy'
  }
  function onDragLeave(e: DragEvent) {
    if (!hasFiles(e)) return
    dragDepth = Math.max(0, dragDepth - 1)
    if (dragDepth === 0) dragOver = false
  }
  function onDrop(e: DragEvent) {
    dragDepth = 0
    dragOver = false
  }

  async function openAbout() {
    try {
      about = await getAbout()
      showAbout = true
    } catch (e: any) {
      error(e?.message ?? 'Could not load About info')
    }
  }
  function closeAbout() {
    showAbout = false
  }
  async function removeRoot(dir: string) {
    const yes = window.confirm(`Remove "${dir}" from your library?\n\nThe files will not be deleted.`)
    if (!yes) return
    try {
      await removeLibrary(dir) // backend binding returns void, NOT settings
      settings = await getSettings() // re-read the authoritative roots
      bump()
      ok('Folder removed — its comics were unindexed')
    } catch (e: any) {
      error(e?.message ?? 'Could not remove folder')
    }
  }
  // Clicking a card now opens the info screen (author, every tag, source) rather
  // than the reader; reading starts from there, and exiting the reader lands back
  // on that screen so the source stays one tap away.
  function openInfo(card: ComicCard) {
    info = { id: card.id, title: card.title }
  }
  function addTag(t: string) {
    t = t.trim()
    if (!t) return
    if (tagFilters.some((x) => x.toLowerCase() === t.toLowerCase())) return
    tagFilters = [...tagFilters, t]
  }
  function removeTag(t: string) {
    tagFilters = tagFilters.filter((x) => x !== t)
  }
  function clearTags() {
    if (tagFilters.length > 0) tagFilters = []
  }
  function closeInfo() {
    info = null
  }
  function startReader(r: ComicRef) {
    reader = r // info is left set: when the reader closes we return to it
  }
  function exitReader() {
    reader = null
    bump() // refresh shelf (progress chips, "recently read" ordering)
  }
</script>

{#if reader}
  <Reader comic={reader} onExit={exitReader} />
{:else if info}
  <ComicInfo comic={info} onExit={closeInfo} onStart={startReader} onAddTag={addTag} />
{:else}
  <TopBar
    roots={settings?.libraryRoots ?? []}
    {scanning}
    progress={scanProgress}
    comics={indexed}
    importing={importing}
    onAdd={addFolder}
    onScan={doScan}
    onCancel={doCancel}
    onRemoveRoot={removeRoot}
    onImportFile={doImportFile}
    onImportFolder={doImportFolder}
    onAbout={openAbout}
    onReloadLayout={doReloadLayout}
  />

  {#if !settings || (settings.libraryRoots ?? []).length === 0}
    <Welcome onAdd={addFolder} />
  {:else if layout}
    <main class="shelf">
      {#each layout.sections as section (section.id)}
        {#if section.kind === 'grid'}
          <Section
            section={section}
            filterRules={filterRules}
            sortRules={sortRules}
            refreshKey={refreshKey}
            scanning={scanning}
            searchText={searchText}
            onSearchChange={(q) => (searchText = q)}
            tags={tagFilters}
            onAddTag={addTag}
            onRemoveTag={removeTag}
            onClearTags={clearTags}
            gridHeight={winH > 0 ? Math.max(300, winH - 123) : 600}
            onOpen={openInfo}
            onChanged={bump}
          />
        {:else}
          <section class="section">
            <div class="head"><h2>{section.title || section.cardId}</h2></div>
            <CardSlot cardId={section.cardId || ''} refreshKey={refreshKey} />
          </section>
        {/if}
      {/each}
    </main>
  {/if}
{/if}

{#if showAbout && about}
  <div class="backdrop" onclick={closeAbout}>
    <div class="modal" onclick={(e) => e.stopPropagation()}>
      <h3>📚 ComicGo</h3>
      <p class="ver">Version {about.version}</p>
      <dl>
        <div class="row"><dt>Comics indexed</dt><dd>{about.comicCount}</dd></div>
        <div class="row"><dt>Media transport</dt><dd>{about.mediaTransport}</dd></div>
        <div class="row"><dt>Data folder</dt><dd class="mono">{about.configDir}</dd></div>
      </dl>
      <div class="actions">
        <button onclick={closeAbout}>Close</button>
      </div>
    </div>
  </div>
{/if}

{#if importing}
  <div class="impbar" role="status">
    <span class="spin"></span>
    <span>{importLabel}</span>
  </div>
{/if}

{#if dragOver}
  <div class="dropover">
    <div class="dropbox">⬇ Drop to import into your library</div>
  </div>
{/if}

<Toasts />

<style>
  .shelf {
    flex: 1 1 auto;
    min-height: 0;
    overflow-y: auto;
    padding-bottom: 4px;
    /* The shelf only scrolls for extra sections; don't show a thin bar for a
       few pixels of overflow — the grid window has its own real scrollbar. */
    scrollbar-width: none;
  }
  .shelf::-webkit-scrollbar {
    display: none;
  }
  .section {
    margin: 14px 18px;
  }
  .head h2 {
    margin: 4px 0;
    font-size: 15px;
    font-weight: 600;
  }
  .backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.45);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 50;
  }
  .modal {
    background: var(--panel);
    border: 1px solid var(--line);
    border-radius: 10px;
    padding: 20px 24px;
    width: 420px;
    max-width: 90vw;
    box-shadow: 0 12px 40px rgba(0, 0, 0, 0.5);
  }
  .modal h3 { margin: 0 0 2px; }
  .ver { color: var(--text-dim); margin: 0 0 12px; font-size: 12px; }
  .modal dl { margin: 0 0 16px; display: flex; flex-direction: column; gap: 6px; }
  .row { display: flex; gap: 10px; font-size: 13px; }
  .row dt { color: var(--text-dim); width: 130px; flex: none; }
  .row dd { margin: 0; }
  .mono { font-family: Consolas, monospace; font-size: 12px; word-break: break-all; }
  .impbar {
    position: fixed;
    top: 62px;
    left: 50%;
    transform: translateX(-50%);
    z-index: 45;
    display: flex;
    align-items: center;
    gap: 9px;
    background: var(--panel);
    border: 1px solid var(--accent);
    color: var(--text);
    font-size: 13px;
    padding: 7px 14px;
    border-radius: 20px;
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.45);
    white-space: nowrap;
    max-width: 80vw;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .spin {
    flex: none;
    width: 13px;
    height: 13px;
    border-radius: 50%;
    border: 2px solid var(--accent-soft);
    border-top-color: var(--accent);
    animation: imp-spin 0.8s linear infinite;
  }
  @keyframes imp-spin { to { transform: rotate(360deg); } }
  .dropover {
    position: fixed;
    inset: 0;
    z-index: 40;
    display: flex;
    align-items: center;
    justify-content: center;
    background: rgba(0, 0, 0, 0.35);
    pointer-events: none; /* the drop lands wherever the cursor is */
  }
  .dropbox {
    border: 2px dashed var(--accent);
    background: rgba(20, 22, 28, 0.85);
    color: var(--accent);
    font-size: 17px;
    font-weight: 600;
    padding: 26px 44px;
    border-radius: 14px;
  }
</style>
