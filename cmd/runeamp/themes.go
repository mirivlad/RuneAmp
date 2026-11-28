package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"regexp"
	"strings"
)

const themeLimit = 4 << 20

var themeColor = regexp.MustCompile("^#[0-9a-fA-F]{6}$")
var paletteKeys = strings.Fields("bg panel line text muted accent primary hover buttonText heroA heroB personal zx ts tracker audio")
var artSlots = strings.Fields("hero personal zx ts tracker audio cover")

type themeManifest struct {
	Version    int               `json:"version"`
	Name       string            `json:"name"`
	Dark       map[string]string `json:"dark"`
	Light      map[string]string `json:"light"`
	Radius     int               `json:"radius"`
	Typography string            `json:"typography"`
	Artwork    string            `json:"artwork"`
	Images     map[string]string `json:"images,omitempty"`
}
type themeInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type themeConfig struct {
	ID       string            `json:"id"`
	Manifest *themeManifest    `json:"manifest,omitempty"`
	Images   map[string]string `json:"images,omitempty"`
}

func (a *App) initThemes() error {
	_, e := a.db.Exec("CREATE TABLE IF NOT EXISTS service_themes(id TEXT PRIMARY KEY,name TEXT NOT NULL,archive BLOB NOT NULL,config BLOB NOT NULL); CREATE TABLE IF NOT EXISTS service_theme_state(slot INTEGER PRIMARY KEY CHECK(slot=1),active TEXT NOT NULL); INSERT OR IGNORE INTO service_theme_state(slot,active) VALUES(1,'default')")
	return e
}
func readTheme(b []byte) (themeManifest, map[string]string, error) {
	var m themeManifest
	bad := func(s string) (themeManifest, map[string]string, error) { return m, nil, errors.New(s) }
	if len(b) > themeLimit {
		return bad("Лимит ZIP 4 МиБ")
	}
	z, e := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if e != nil || len(z.File) > 8 {
		return bad("Некорректный ZIP темы")
	}
	files := map[string][]byte{}
	total := uint64(0)
	for _, f := range z.File {
		n := f.Name
		if !f.Mode().IsRegular() || strings.ContainsAny(n, "/\\:") || n == "." || n == ".." || files[n] != nil {
			return bad("Недопустимый путь или дубликат")
		}
		total += f.UncompressedSize64
		if f.UncompressedSize64 > 2<<20 || total > themeLimit {
			return bad("Слишком большой архив темы")
		}
		r, e := f.Open()
		if e != nil {
			return bad("Повреждённый архив")
		}
		d, e := io.ReadAll(io.LimitReader(r, 2<<20+1))
		r.Close()
		if e != nil || uint64(len(d)) != f.UncompressedSize64 {
			return bad("Повреждённый файл")
		}
		files[n] = d
	}
	raw, ok := files["theme.json"]
	if !ok || len(raw) > 16384 {
		return bad("Нет theme.json или он слишком большой")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if dec.Decode(&m) != nil || dec.Decode(new(any)) != io.EOF || m.Version != 1 || strings.TrimSpace(m.Name) == "" || len(m.Name) > 160 || m.Radius < 0 || m.Radius > 16 {
		return bad("Некорректный манифест темы")
	}
	for _, p := range []map[string]string{m.Dark, m.Light} {
		if len(p) != len(paletteKeys) {
			return bad("Неполная палитра")
		}
		for _, k := range paletteKeys {
			if !themeColor.MatchString(p[k]) {
				return bad("Некорректный цвет: " + k)
			}
		}
	}
	if m.Typography != "system" && m.Typography != "mono" && m.Typography != "serif" {
		return bad("Недопустимая типографика")
	}
	if m.Artwork != "industrial" && m.Artwork != "geometry" && m.Artwork != "records" {
		return bad("Недопустимый графический стиль")
	}
	images := map[string]string{}
	used := map[string]bool{"theme.json": true}
	for slot, n := range m.Images {
		valid := false
		for _, s := range artSlots {
			if s == slot {
				valid = true
			}
		}
		d, ok := files[n]
		if !valid || !ok || n == "theme.json" {
			return bad("Недопустимое изображение")
		}
		cfg, format, e := image.DecodeConfig(bytes.NewReader(d))
		if e != nil || (format != "png" && format != "jpeg") || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 2048 || cfg.Height > 2048 || cfg.Width*cfg.Height > 4194304 {
			return bad("Ожидается PNG/JPEG до 2048×2048")
		}
		if _, _, e = image.Decode(bytes.NewReader(d)); e != nil {
			return bad("Повреждённое изображение")
		}
		images[slot] = "data:image/" + format + ";base64," + base64.StdEncoding.EncodeToString(d)
		used[n] = true
	}
	if len(used) != len(files) {
		return bad("Архив содержит лишние файлы; CSS, JS и HTML запрещены")
	}
	return m, images, nil
}
func (a *App) activeTheme(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	fallback := themeConfig{ID: "default"}
	var b []byte
	if a.db.QueryRow("SELECT t.config FROM service_themes t JOIN service_theme_state s ON t.id=s.active WHERE s.slot=1").Scan(&b) != nil || !json.Valid(b) {
		respond(w, fallback)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(b)
}
func (a *App) themeList(w http.ResponseWriter, r *http.Request) {
	rows, e := a.db.Query("SELECT id,name FROM service_themes ORDER BY name,id")
	if e != nil {
		fail(w, 500, "Ошибка БД")
		return
	}
	defer rows.Close()
	out := []themeInfo{{"default", "Industrial Music Archive (заводская)"}}
	for rows.Next() {
		var t themeInfo
		if rows.Scan(&t.ID, &t.Name) != nil {
			fail(w, 500, "Ошибка БД")
			return
		}
		out = append(out, t)
	}
	if rows.Err() != nil {
		fail(w, 500, "Ошибка БД")
		return
	}
	respond(w, out)
}
func (a *App) themeUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, themeLimit+65536)
	mr, e := r.MultipartReader()
	if e != nil {
		fail(w, 400, "Ожидается ZIP")
		return
	}
	p, e := mr.NextPart()
	if e != nil {
		fail(w, 400, "Нет файла")
		return
	}
	defer p.Close()
	b, e := io.ReadAll(io.LimitReader(p, themeLimit+1))
	if e != nil || len(b) > themeLimit {
		fail(w, 400, "Лимит ZIP 4 МиБ")
		return
	}
	m, images, e := readTheme(b)
	if e != nil {
		fail(w, 422, e.Error())
		return
	}
	hash := sha256.Sum256(b)
	id := hex.EncodeToString(hash[:16])
	config, e := json.Marshal(themeConfig{id, &m, images})
	if e != nil {
		fail(w, 500, "Ошибка темы")
		return
	}
	tx, e := a.db.Begin()
	if e != nil {
		fail(w, 500, "Ошибка БД")
		return
	}
	defer tx.Rollback()
	var count int
	if tx.QueryRow("SELECT count(*) FROM service_themes").Scan(&count) != nil || count >= 32 {
		fail(w, 409, "Лимит 32 тем")
		return
	}
	if _, e = tx.Exec("INSERT INTO service_themes(id,name,archive,config) VALUES(?,?,?,?)", id, m.Name, b, config); e != nil {
		fail(w, 409, "Тема уже существует или недоступно хранилище")
		return
	}
	if tx.Commit() != nil {
		fail(w, 500, "Ошибка БД")
		return
	}
	respond(w, themeInfo{id, m.Name})
}
func (a *App) themeApply(w http.ResponseWriter, r *http.Request) {
	var v struct {
		ID string `json:"id"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&v) != nil {
		fail(w, 400, "Некорректный запрос")
		return
	}
	tx, e := a.db.Begin()
	if e != nil {
		fail(w, 500, "Ошибка БД")
		return
	}
	defer tx.Rollback()
	if v.ID != "default" {
		var b []byte
		if tx.QueryRow("SELECT archive FROM service_themes WHERE id=?", v.ID).Scan(&b) != nil {
			fail(w, 404, "Тема не найдена")
			return
		}
		if _, _, e = readTheme(b); e != nil {
			fail(w, 422, e.Error())
			return
		}
	}
	if _, e = tx.Exec("UPDATE service_theme_state SET active=? WHERE slot=1", v.ID); e != nil || tx.Commit() != nil {
		fail(w, 500, "Ошибка БД")
		return
	}
	respond(w, v)
}
func (a *App) themeDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "default" {
		fail(w, 400, "Заводскую тему удалить нельзя")
		return
	}
	tx, e := a.db.Begin()
	if e != nil {
		fail(w, 500, "Ошибка БД")
		return
	}
	defer tx.Rollback()
	if _, e = tx.Exec("UPDATE service_theme_state SET active='default' WHERE active=?", id); e != nil {
		fail(w, 500, "Ошибка БД")
		return
	}
	if _, e = tx.Exec("DELETE FROM service_themes WHERE id=?", id); e != nil || tx.Commit() != nil {
		fail(w, 500, "Ошибка БД")
		return
	}
	respond(w, map[string]bool{"ok": true})
}
