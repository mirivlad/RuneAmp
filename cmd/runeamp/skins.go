package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
)

const skinLimit = 8 << 20

type skinInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Size    int    `json:"size"`
	Visible bool   `json:"visible"`
	Default bool   `json:"default"`
}

func (a *App) initSkins() error {
	_, e := a.db.Exec("CREATE TABLE IF NOT EXISTS skins(id TEXT PRIMARY KEY,name TEXT NOT NULL,archive BLOB NOT NULL,visible INTEGER NOT NULL DEFAULT 1,preferred INTEGER NOT NULL DEFAULT 0)")
	return e
}
func validateSkin(b []byte) error {
	z, e := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if e != nil {
		return e
	}
	if len(z.File) > 2000 {
		return errors.New("Слишком много записей")
	}
	allowed := strings.Fields("main.bmp cbuttons.bmp titlebar.bmp pledit.bmp pledit.txt viscolor.txt volume.bmp posbar.bmp shufrep.bmp numbers.bmp nums_ex.bmp text.bmp playpaus.bmp balance.bmp monoster.bmp")
	seen := map[string]bool{}
	total := uint64(0)
	for _, f := range z.File {
		n := strings.ToLower(path.Base(strings.ReplaceAll(f.Name, "\\", "/")))
		ok := false
		for _, v := range allowed {
			if n == v {
				ok = true
				break
			}
		}
		if !ok {
			continue
		}
		if seen[n] || !f.Mode().IsRegular() {
			return errors.New("Некорректная запись скина")
		}
		seen[n] = true
		total += f.UncompressedSize64
		if f.UncompressedSize64 > 2<<20 || total > skinLimit {
			return errors.New("Слишком большой скин")
		}
		r, e := f.Open()
		if e != nil {
			return e
		}
		d, e := io.ReadAll(io.LimitReader(r, 2<<20+1))
		r.Close()
		if e != nil || uint64(len(d)) != f.UncompressedSize64 {
			return errors.New("Повреждённый скин")
		}
		if strings.HasSuffix(n, ".bmp") {
			if len(d) < 26 || string(d[:2]) != "BM" {
				return errors.New("Некорректный BMP")
			}
			var w, h int64
			if binary.LittleEndian.Uint32(d[14:]) == 12 {
				w = int64(binary.LittleEndian.Uint16(d[18:]))
				h = int64(binary.LittleEndian.Uint16(d[20:]))
			} else {
				w = int64(int32(binary.LittleEndian.Uint32(d[18:])))
				h = int64(int32(binary.LittleEndian.Uint32(d[22:])))
				if h < 0 {
					h = -h
				}
			}
			if w < 1 || h < 1 || w > 2048 || h > 2048 || w*h > 2000000 || n == "main.bmp" && (w < 275 || h < 116) {
				return errors.New("Некорректные размеры BMP")
			}
		}
	}
	if !seen["main.bmp"] {
		return errors.New("Нет MAIN.BMP")
	}
	return nil
}
func (a *App) skinList(admin bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := "SELECT id,name,length(archive),visible,preferred FROM skins"
		if !admin {
			q += " WHERE visible=1"
		}
		q += " ORDER BY name"
		rows, e := a.db.Query(q)
		if e != nil {
			fail(w, 500, "Ошибка БД")
			return
		}
		defer rows.Close()
		out := []skinInfo{}
		for rows.Next() {
			var s skinInfo
			if rows.Scan(&s.ID, &s.Name, &s.Size, &s.Visible, &s.Default) != nil {
				fail(w, 500, "Ошибка БД")
				return
			}
			out = append(out, s)
		}
		if rows.Err() != nil {
			fail(w, 500, "Ошибка БД")
			return
		}
		respond(w, out)
	}
}
func (a *App) skinDownload(admin bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := "SELECT archive FROM skins WHERE id=?"
		if !admin {
			q += " AND visible=1"
		}
		var b []byte
		if a.db.QueryRow(q, r.PathValue("id")).Scan(&b) != nil {
			fail(w, 404, "Скин не найден")
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", "attachment; filename=skin.wsz")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Length", strconv.Itoa(len(b)))
		w.Write(b)
	}
}
func (a *App) skinUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, skinLimit+65536)
	mr, e := r.MultipartReader()
	if e != nil {
		fail(w, 400, "Ожидается архив")
		return
	}
	p, e := mr.NextPart()
	if e != nil {
		fail(w, 400, "Нет файла")
		return
	}
	defer p.Close()
	name := p.FileName()
	ext := strings.ToLower(path.Ext(name))
	if len(name) > 240 || ext != ".wsz" && ext != ".zip" {
		fail(w, 400, "Ожидается WSZ/ZIP")
		return
	}
	b, e := io.ReadAll(io.LimitReader(p, skinLimit+1))
	if e != nil || len(b) > skinLimit {
		fail(w, 400, "Лимит 8 МиБ")
		return
	}
	if e = validateSkin(b); e != nil {
		fail(w, 422, e.Error())
		return
	}
	hash := sha256.Sum256(b)
	id := hex.EncodeToString(hash[:16])
	tx, e := a.db.Begin()
	if e != nil {
		fail(w, 500, "Ошибка БД")
		return
	}
	defer tx.Rollback()
	var count int
	if tx.QueryRow("SELECT count(*) FROM skins").Scan(&count) != nil || count >= 100 {
		fail(w, 409, "Лимит 100 скинов")
		return
	}
	if _, e = tx.Exec("INSERT INTO skins(id,name,archive) VALUES(?,?,?)", id, strings.TrimSuffix(name, path.Ext(name)), b); e != nil {
		fail(w, 409, "Скин уже существует или недоступно хранилище")
		return
	}
	if tx.Commit() != nil {
		fail(w, 500, "Ошибка БД")
		return
	}
	respond(w, skinInfo{id, strings.TrimSuffix(name, path.Ext(name)), len(b), true, false})
}
func (a *App) skinUpdate(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var p struct {
		Visible bool
		Default bool
	}
	if json.NewDecoder(r.Body).Decode(&p) != nil || p.Default && !p.Visible {
		fail(w, 400, "Некорректные параметры")
		return
	}
	tx, e := a.db.Begin()
	if e != nil {
		fail(w, 500, "Ошибка БД")
		return
	}
	defer tx.Rollback()
	if p.Default {
		if _, e = tx.Exec("UPDATE skins SET preferred=0"); e != nil {
			fail(w, 500, "Ошибка БД")
			return
		}
	}
	res, e := tx.Exec("UPDATE skins SET visible=?,preferred=? WHERE id=?", p.Visible, p.Default, r.PathValue("id"))
	if e != nil {
		fail(w, 500, "Ошибка БД")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		fail(w, 404, "Скин не найден")
		return
	}
	if tx.Commit() != nil {
		fail(w, 500, "Ошибка БД")
		return
	}
	respond(w, map[string]bool{"ok": true})
}
func (a *App) skinDelete(w http.ResponseWriter, r *http.Request) {
	if _, e := a.db.Exec("DELETE FROM skins WHERE id=?", r.PathValue("id")); e != nil {
		fail(w, 500, "Ошибка БД")
		return
	}
	respond(w, map[string]bool{"ok": true})
}
