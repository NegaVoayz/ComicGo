// dbcheck prints what the app currently has on disk: settings roots, indexed
// comic count, and a few sample rows. Useful for support/debugging.
package main

import (
	"fmt"
	"os"

	"comicgo/internal/config"
	"comicgo/internal/index"
)

func main() {
	s, err := config.LoadSettings()
	if err != nil {
		fmt.Println("settings error:", err)
	} else {
		fmt.Println("roots:", s.LibraryRoots)
		fmt.Println("transport:", s.MediaTransport)
	}
	paths, err := config.Resolve()
	if err != nil {
		fmt.Println("resolve error:", err)
		os.Exit(1)
	}
	fmt.Println("db:", paths.DB)
	st, err := index.Open(paths.DB)
	if err != nil {
		fmt.Println("open db error:", err)
		os.Exit(1)
	}
	defer st.Close()
	rows, err := st.AllRows()
	if err != nil {
		fmt.Println("read error:", err)
		os.Exit(1)
	}
	fmt.Println("indexed comics:", len(rows))
	for i := range rows {
		r := &rows[i]
		title := r.KV["Title"]
		if title == "" {
			title = r.Filename
		}
		fmt.Printf("  %4d  pages=%-3d nonUTF8=%-5v meta=%d  %s\n", r.ID, r.PageCount, r.HasNonUTF8, len(r.KV), title)
		if i >= 9 {
			fmt.Println("  ...")
			break
		}
	}
	if len(rows) == 0 {
		fmt.Println("(no comics indexed — check the root path and that files end in .zip/.cbz)")
	}
}
