package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

func (a *App) appendPlaylist(ac *Accounts, public bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := ac.who(r)
		if id.Role == "guest" {
			fail(w, 401, "Войдите")
			return
		}
		if public && id.Role != "admin" {
			fail(w, 403, "Нет прав")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		var p struct {
			Name   string
			IDs    []string
			Create bool
		}
		if json.NewDecoder(r.Body).Decode(&p) != nil || strings.TrimSpace(p.Name) == "" || len(p.Name) > 120 || len(p.IDs) == 0 || len(p.IDs) > 10000 {
			fail(w, 400, "Некорректный плейлист")
			return
		}
		a.scanMu.Lock()
		defer a.scanMu.Unlock()
		a.mu.RLock()
		for _, v := range p.IDs {
			if _, ok := a.tracks[v]; !ok {
				a.mu.RUnlock()
				fail(w, 409, "Трек отсутствует в библиотеке")
				return
			}
		}
		a.mu.RUnlock()
		tx, e := a.db.BeginTx(r.Context(), nil)
		if e != nil {
			fail(w, 500, "Ошибка БД")
			return
		}
		defer tx.Rollback()
		var raw string
		if public {
			e = tx.QueryRow("SELECT ids FROM playlists WHERE name=?", p.Name).Scan(&raw)
		} else {
			e = tx.QueryRow("SELECT ids FROM personal_playlists WHERE owner=? AND name=?", id.Name, p.Name).Scan(&raw)
		}
		ids := []string{}
		if errors.Is(e, sql.ErrNoRows) {
			if !p.Create {
				fail(w, 404, "Плейлист не найден")
				return
			}
		} else if e != nil || json.Unmarshal([]byte(raw), &ids) != nil {
			fail(w, 500, "Ошибка БД")
			return
		} else if p.Create {
			fail(w, 409, "Плейлист уже существует")
			return
		}
		seen := map[string]bool{}
		for _, v := range ids {
			seen[v] = true
		}
		added, skipped := 0, 0
		for _, v := range p.IDs {
			if seen[v] {
				skipped++
				continue
			}
			seen[v] = true
			ids = append(ids, v)
			added++
		}
		if len(ids) > 10000 {
			fail(w, 400, "Лимит 10000 треков")
			return
		}
		b, _ := json.Marshal(ids)
		if public {
			_, e = tx.Exec("INSERT INTO playlists(name,ids) VALUES(?,?) ON CONFLICT(name) DO UPDATE SET ids=excluded.ids", p.Name, string(b))
		} else {
			_, e = tx.Exec("INSERT INTO personal_playlists(owner,name,ids) VALUES(?,?,?) ON CONFLICT(owner,name) DO UPDATE SET ids=excluded.ids", id.Name, p.Name, string(b))
		}
		if e != nil || tx.Commit() != nil {
			fail(w, 500, "Ошибка сохранения")
			return
		}
		respond(w, map[string]any{"added": added, "skipped": skipped, "playlist": Playlist{p.Name, ids}})
	}
}
