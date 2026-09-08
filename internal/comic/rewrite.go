package comic

import (
	"archive/zip"
	"errors"
	"fmt"
	"os"
)

// maxCommentLen is the zip EOCD comment field limit (uint16). Builders that
// want a clearer error message than the stdlib's can check it up front.
const maxCommentLen = 0xffff

// RewriteComment replaces the zip comment of an existing .cbz/.zip without
// touching the entry contents: every member is raw-copied (no recompression,
// original names/order/methods and any non-UTF-8 names preserved) into a
// sibling temp file whose comment is set to comment, the temp file is
// validated, and then it is atomically renamed over path. The original file is
// never left partially written — on any error the temp file is removed and the
// original is untouched.
func RewriteComment(path, comment string) error {
	if len(comment) > maxCommentLen {
		return errors.New("comic: comment too long (max 65535 bytes)")
	}

	rc, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	rcClosed := false
	defer func() {
		if !rcClosed {
			_ = rc.Close()
		}
	}()

	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	fail := func(werr error) error {
		_ = f.Close()
		_ = os.Remove(tmp)
		return werr
	}

	zw := zip.NewWriter(f)
	if err := zw.SetComment(comment); err != nil {
		return fail(err)
	}
	for _, entry := range rc.File {
		if err := zw.Copy(entry); err != nil {
			return fail(fmt.Errorf("copy %q: %w", entry.Name, err))
		}
	}
	if err := zw.Close(); err != nil {
		return fail(err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}

	// Prove the temp archive is well-formed (comment + entry count) before it
	// replaces the original, so a corrupt rewrite can never destroy a good file.
	if err := verifyRewrite(tmp, comment, len(rc.File)); err != nil {
		_ = os.Remove(tmp)
		return err
	}

	// Release the read handle on the original first: on Windows a file that is
	// open cannot be replaced by os.Rename.
	if err := rc.Close(); err != nil {
		rcClosed = true
		_ = os.Remove(tmp)
		return err
	}
	rcClosed = true

	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func verifyRewrite(tmp, comment string, wantEntries int) error {
	zr, err := zip.OpenReader(tmp)
	if err != nil {
		return fmt.Errorf("rewritten archive unreadable: %w", err)
	}
	defer zr.Close()
	if zr.Comment != comment {
		return errors.New("rewritten archive comment mismatch")
	}
	if len(zr.File) != wantEntries {
		return fmt.Errorf("rewritten entry count = %d, want %d", len(zr.File), wantEntries)
	}
	return nil
}
