package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"path"
	"strings"

	"golang.org/x/sys/unix"
)

func (a *App) initIDs() error {
	if _, e := a.db.Exec("CREATE TABLE IF NOT EXISTS track_paths(source TEXT NOT NULL,path TEXT NOT NULL,id TEXT NOT NULL,PRIMARY KEY(source,path))"); e != nil {
		return e
	}
	rows, e := a.db.Query("SELECT source,path,id FROM track_paths")
	if e != nil {
		return e
	}
	defer rows.Close()
	m := map[string]string{}
	for rows.Next() {
		var s, p, id string
		if e = rows.Scan(&s, &p, &id); e != nil {
			return e
		}
		m[s+"\x00"+p] = id
	}
	if e = rows.Err(); e != nil {
		return e
	}
	a.pathIDs = m
	return nil
}
func (a *App) resolveID(t Track) Track {
	if id := a.pathIDs[t.Source+"\x00"+t.Path]; id != "" {
		t.ID = id
	}
	return t
}
func (a *App) moveEntry(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	var p struct{ Source, Path, Destination string }
	if json.NewDecoder(r.Body).Decode(&p) != nil || p.Path == "." || p.Destination == "." || !validDirectory(p.Path) || !validDirectory(p.Destination) || p.Path == p.Destination || strings.HasPrefix(p.Destination, p.Path+"/") {
		fail(w, 400, "Некорректный путь")
		return
	}
	root, ok := a.roots[p.Source]
	if !ok || p.Source == "music" && !a.musicWritable {
		fail(w, 403, "Хранилище недоступно для записи")
		return
	}
	a.scanMu.Lock()
	defer a.scanMu.Unlock()
	if a.pathIDs == nil {
		if e := a.initIDs(); e != nil {
			fail(w, 500, "Ошибка БД")
			return
		}
	}
	if !safeDirectory(root, path.Dir(p.Path)) || !safeDirectory(root, path.Dir(p.Destination)) {
		fail(w, 400, "Папка недоступна")
		return
	}
	from, e := root.Open(path.Dir(p.Path))
	if e != nil {
		fail(w, 400, "Папка недоступна")
		return
	}
	defer from.Close()
	to, e := root.Open(path.Dir(p.Destination))
	if e != nil {
		fail(w, 400, "Папка недоступна")
		return
	}
	defer to.Close()
	info, e := root.Lstat(p.Path)
	if e != nil || !info.IsDir() && !info.Mode().IsRegular() {
		fail(w, 409, "Объект недоступен")
		return
	}
	if !info.IsDir() && !strings.EqualFold(path.Ext(p.Path), path.Ext(p.Destination)) {
		fail(w, 400, "Расширение файла менять нельзя")
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	tx, e := a.db.Begin()
	if e != nil {
		fail(w, 500, "Ошибка БД")
		return
	}
	defer tx.Rollback()
	changes := map[string]string{}
	updated := map[string]Track{}
	for id, t := range a.tracks {
		if t.Source != p.Source || !(t.Path == p.Path || info.IsDir() && strings.HasPrefix(t.Path, p.Path+"/")) {
			continue
		}
		newPath := p.Destination + strings.TrimPrefix(t.Path, p.Path)
		if !validDirectory(newPath) {
			fail(w, 400, "Слишком длинный путь")
			return
		}
		var b [16]byte
		if _, e = rand.Read(b[:]); e != nil {
			fail(w, 500, "Ошибка ID")
			return
		}
		for key, value := range map[string]string{t.Path: hex.EncodeToString(b[:]), newPath: id} {
			if _, e = tx.Exec("INSERT INTO track_paths(source,path,id) VALUES(?,?,?) ON CONFLICT(source,path) DO UPDATE SET id=excluded.id", p.Source, key, value); e != nil {
				fail(w, 500, "Ошибка БД")
				return
			}
			changes[p.Source+"\x00"+key] = value
		}
		t.Path = newPath
		t.Name = path.Base(newPath)
		updated[id] = t
	}
	if e = unix.Renameat2(int(from.Fd()), path.Base(p.Path), int(to.Fd()), path.Base(p.Destination), unix.RENAME_NOREPLACE); e != nil {
		fail(w, 409, "Не удалось переместить: "+e.Error())
		return
	}
	if e = tx.Commit(); e != nil {
		if re := unix.Renameat2(int(to.Fd()), path.Base(p.Destination), int(from.Fd()), path.Base(p.Path), unix.RENAME_NOREPLACE); re != nil {
			fail(w, 500, "Ошибка БД и отката перемещения; проверьте каталог")
			return
		}
		fail(w, 500, "Ошибка БД, перемещение отменено")
		return
	}
	for k, v := range changes {
		a.pathIDs[k] = v
	}
	for id, t := range updated {
		a.tracks[id] = t
	}
	respond(w, map[string]any{"path": p.Destination, "tracks": len(updated)})
}
func (a *App) uploadSource(r *http.Request) (string, bool) {
	s := r.URL.Query().Get("source")
	if s == "" {
		s = "uploads"
		if a.musicWritable {
			s = "music"
		}
	}
	return s, s == "uploads" || s == "music" && a.musicWritable
}
