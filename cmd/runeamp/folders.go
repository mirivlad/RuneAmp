package main

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func validDirectory(dir string) bool {
	if dir == "." {
		return true
	}
	if !fs.ValidPath(dir) || len(dir) > 1024 || strings.ContainsAny(dir, "\\\x00") {
		return false
	}
	for _, part := range strings.Split(dir, "/") {
		if strings.HasPrefix(part, ".") || len(part) > 240 {
			return false
		}
	}
	return true
}
func safeDirectory(root *os.Root, dir string) bool {
	if !validDirectory(dir) {
		return false
	}
	if dir == "." {
		return true
	}
	p := ""
	for _, part := range strings.Split(dir, "/") {
		p = filepath.Join(p, part)
		s, e := root.Lstat(p)
		if e != nil || !s.IsDir() {
			return false
		}
	}
	return true
}
func (a *App) folders(w http.ResponseWriter, r *http.Request) {
	dirs := []string{"."}
	if a.musicWritable {
		e := fs.WalkDir(a.roots["music"].FS(), ".", func(p string, d fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if d.IsDir() && p != "." {
				if !validDirectory(p) {
					return fs.SkipDir
				}
				if len(dirs) >= 10000 {
					return fs.SkipAll
				}
				dirs = append(dirs, p)
			}
			return nil
		})
		if e != nil {
			fail(w, 500, "Ошибка чтения папок")
			return
		}
	}
	sort.Strings(dirs)
	respond(w, map[string]any{"writable": a.musicWritable, "folders": dirs})
}
func (a *App) mkdir(w http.ResponseWriter, r *http.Request) {
	if !a.musicWritable {
		fail(w, 403, "Каталог только для чтения")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var p struct{ Path string }
	if json.NewDecoder(r.Body).Decode(&p) != nil || p.Path == "." || !validDirectory(p.Path) {
		fail(w, 400, "Некорректная папка")
		return
	}
	root := a.roots["music"]
	a.scanMu.Lock()
	defer a.scanMu.Unlock()
	if !safeDirectory(root, filepath.Dir(p.Path)) {
		fail(w, 400, "Родительская папка недоступна")
		return
	}
	if e := root.Mkdir(p.Path, 0755); e != nil {
		fail(w, 409, "Не удалось создать папку")
		return
	}
	respond(w, map[string]string{"path": p.Path})
}
