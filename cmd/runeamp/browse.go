package main

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type FolderEntry struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Directory bool   `json:"directory"`
	Format    string `json:"format,omitempty"`
	Size      int64  `json:"size,omitempty"`
	Indexed   bool   `json:"indexed"`
	Music     bool   `json:"music"`
}

func (a *App) folderContents(w http.ResponseWriter, r *http.Request) {
	source := r.URL.Query().Get("source")
	if source == "" {
		source = "music"
	}
	root, ok := a.roots[source]
	dir := r.URL.Query().Get("dir")
	if dir == "" {
		dir = "."
	}
	if !ok || !safeDirectory(root, dir) {
		fail(w, 400, "Папка недоступна")
		return
	}
	f, e := root.Open(dir)
	if e != nil {
		fail(w, 400, "Папка недоступна")
		return
	}
	defer f.Close()
	entries, e := f.ReadDir(10001)
	if e != nil && e != io.EOF {
		fail(w, 500, "Ошибка чтения")
		return
	}
	out := []FolderEntry{}
	tracks, folders := 0, 0
	for _, d := range entries {
		if strings.HasPrefix(d.Name(), ".") || d.Type()&os.ModeSymlink != 0 {
			continue
		}
		path := filepath.ToSlash(filepath.Join(dir, d.Name()))
		if d.IsDir() {
			folders++
			out = append(out, FolderEntry{Name: d.Name(), Path: path, Directory: true})
			continue
		}
		if !d.Type().IsRegular() {
			continue
		}
		info, e := d.Info()
		if e != nil {
			continue
		}
		t, ok := makeTrack(source, path, info.Size())
		if !ok {
			out = append(out, FolderEntry{Name: d.Name(), Path: path, Size: info.Size()})
			continue
		}
		tracks++
		a.mu.RLock()
		t = a.resolveID(t)
		_, indexed := a.tracks[t.ID]
		a.mu.RUnlock()
		out = append(out, FolderEntry{Name: d.Name(), Path: path, Format: t.Format, Size: t.Size, Indexed: indexed, Music: true})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Directory != out[j].Directory {
			return out[i].Directory
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	respond(w, map[string]any{"writable": source == "uploads" || a.musicWritable, "entries": out, "tracks": tracks, "folders": folders, "truncated": len(entries) > 10000})
}
