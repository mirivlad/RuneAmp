package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"path"
	"strings"
)

type deleteEntryRequest struct {
	Source    string `json:"source"`
	Path      string `json:"path"`
	Directory bool   `json:"directory"`
}

func (a *App) deleteEntry(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	var p deleteEntryRequest
	if json.NewDecoder(r.Body).Decode(&p) != nil || p.Path == "." || !validDirectory(p.Path) {
		fail(w, 400, "Некорректный путь")
		return
	}
	root, ok := a.roots[p.Source]
	if !ok {
		fail(w, 400, "Неизвестное хранилище")
		return
	}
	if p.Source == "music" && !a.musicWritable {
		fail(w, 403, "Музыкальный каталог только для чтения")
		return
	}
	a.scanMu.Lock()
	defer a.scanMu.Unlock()
	dir, base := path.Dir(p.Path), path.Base(p.Path)
	if !safeDirectory(root, dir) {
		fail(w, 400, "Родительская папка недоступна")
		return
	}
	parent, e := root.OpenRoot(dir)
	if e != nil {
		fail(w, 400, "Родительская папка недоступна")
		return
	}
	defer parent.Close()
	info, e := parent.Lstat(base)
	if errors.Is(e, fs.ErrNotExist) {
		fail(w, 404, "Файл или папка уже отсутствует")
		return
	}
	if e != nil || info.IsDir() != p.Directory || (!info.IsDir() && !info.Mode().IsRegular()) {
		fail(w, 409, "Объект изменился или недоступен")
		return
	}
	if p.Directory {
		e = parent.RemoveAll(base)
	} else {
		e = parent.Remove(base)
	}
	// Reconcile the affected part of the index even after a partial filesystem error.
	a.mu.Lock()
	removed := 0
	for id, t := range a.tracks {
		if t.Source != p.Source || !(t.Path == p.Path || p.Directory && strings.HasPrefix(t.Path, p.Path+"/")) {
			continue
		}
		if e != nil {
			if _, err := root.Lstat(t.Path); !errors.Is(err, fs.ErrNotExist) {
				continue
			}
		}
		delete(a.tracks, id)
		removed++
	}
	a.mu.Unlock()
	if e != nil {
		log.Printf("music deletion %s/%s: %v", p.Source, p.Path, e)
		fail(w, 500, "Не удалось удалить всё содержимое. Список обновлён; проверьте права доступа.")
		return
	}
	respond(w, map[string]any{"removed": removed, "path": p.Path})
}
