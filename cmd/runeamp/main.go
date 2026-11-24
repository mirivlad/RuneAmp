package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	_ "modernc.org/sqlite"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Track struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Path   string `json:"path"`
	Source string `json:"source"`
	Format string `json:"format"`
	Size   int64  `json:"size"`
	Mode   string `json:"mode"`
}
type Playlist struct {
	Name string   `json:"name"`
	IDs  []string `json:"ids"`
}
type App struct {
	roots         map[string]*os.Root
	mu            sync.RWMutex
	scanMu        sync.Mutex
	tracks        map[string]Track
	db            *sql.DB
	slots         chan struct{}
	uploadSlots   chan struct{}
	musicWritable bool
}

var formats = map[string]string{"mp3": "native", "flac": "native", "wav": "native", "ogg": "native", "opus": "native", "m4a": "native", "aac": "convert", "aiff": "convert", "wma": "convert", "pt2": "ay", "pt3": "ay", "stp": "ay", "mod": "module", "xm": "module", "s3m": "module", "it": "module", "mptm": "module", "med": "module", "stm": "module", "mtm": "module", "669": "module", "ult": "module", "far": "module", "nsf": "gme", "spc": "gme", "gbs": "gme", "vgm": "gme", "vgz": "gme", "ay": "gme"}

const maxUpload = 256 << 20

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func trackID(source, path string) string {
	sum := sha256.Sum256([]byte(source + "\x00" + path))
	return hex.EncodeToString(sum[:16])
}
func makeTrack(source, path string, size int64) (Track, bool) {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	mode, ok := formats[ext]
	if !ok || size <= 0 || (mode == "ay" && size > 65536) {
		return Track{}, false
	}
	return Track{trackID(source, path), filepath.Base(path), path, source, ext, size, mode}, true
}
func (a *App) scan() error {
	a.scanMu.Lock()
	defer a.scanMu.Unlock()
	next := map[string]Track{}
	for source, root := range a.roots {
		err := fs.WalkDir(root.FS(), ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.Type()&os.ModeSymlink != 0 {
				return nil
			}
			if d.IsDir() {
				if path != "." && strings.HasPrefix(d.Name(), ".") {
					return fs.SkipDir
				}
				return nil
			}
			if !d.Type().IsRegular() || strings.HasPrefix(d.Name(), ".") {
				return nil
			}
			info, e := d.Info()
			if e != nil {
				return e
			}
			if t, ok := makeTrack(source, path, info.Size()); ok {
				if len(next) >= 100000 {
					return errors.New("library exceeds 100000 tracks")
				}
				next[t.ID] = t
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	a.mu.Lock()
	a.tracks = next
	a.mu.Unlock()
	return nil
}
func respond(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, s string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": s})
}
func (a *App) library(w http.ResponseWriter, r *http.Request) {
	a.mu.RLock()
	ts := make([]Track, 0, len(a.tracks))
	for _, t := range a.tracks {
		ts = append(ts, t)
	}
	a.mu.RUnlock()
	sort.Slice(ts, func(i, j int) bool {
		if ts[i].Source != ts[j].Source {
			return ts[i].Source < ts[j].Source
		}
		return strings.ToLower(ts[i].Path) < strings.ToLower(ts[j].Path)
	})
	respond(w, ts)
}
func (a *App) rescan(w http.ResponseWriter, r *http.Request) {
	if err := a.scan(); err != nil {
		log.Print(err)
		fail(w, 500, "Ошибка сканирования")
		return
	}
	a.library(w, r)
}
func (a *App) get(id string) (Track, *os.File, error) {
	a.mu.RLock()
	t, ok := a.tracks[id]
	a.mu.RUnlock()
	if !ok {
		return t, nil, os.ErrNotExist
	}
	f, err := a.roots[t.Source].Open(t.Path)
	if err != nil {
		return t, nil, err
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		f.Close()
		return t, nil, os.ErrNotExist
	}
	return t, f, nil
}
func (a *App) media(w http.ResponseWriter, r *http.Request) {
	t, f, err := a.get(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if t.Mode == "ay" {
		if st.Size() > 65536 {
			fail(w, 400, "Модуль слишком большой")
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
	} else {
		types := map[string]string{"mp3": "audio/mpeg", "flac": "audio/flac", "wav": "audio/wav", "ogg": "audio/ogg", "opus": "audio/ogg", "m4a": "audio/mp4"}
		if v := types[t.Format]; v != "" {
			w.Header().Set("Content-Type", v)
		} else {
			w.Header().Set("Content-Type", "application/octet-stream")
		}
	}
	http.ServeContent(w, r, t.Name, st.ModTime(), f)
}
func (a *App) stream(w http.ResponseWriter, r *http.Request) {
	t, f, err := a.get(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	if t.Mode == "ay" {
		fail(w, 400, "AY plays in browser")
		return
	}
	select {
	case a.slots <- struct{}{}:
		defer func() { <-a.slots }()
	default:
		fail(w, 503, "Все декодеры заняты")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 35*time.Minute)
	defer cancel()
	args := []string{"-nostdin", "-v", "error", "-protocol_whitelist", "file,pipe", "-threads", "1"}
	if t.Mode == "module" {
		args = append(args, "-f", "libopenmpt")
	} else if t.Mode == "gme" {
		args = append(args, "-f", "libgme")
	} else {
		// Do not let a renamed playlist or manifest select an arbitrary demuxer.
		demuxers := map[string]string{"mp3": "mp3", "flac": "flac", "wav": "wav", "ogg": "ogg", "opus": "ogg", "m4a": "mov", "aac": "aac", "aiff": "aiff", "wma": "asf"}
		demuxer, ok := demuxers[t.Format]
		if !ok {
			fail(w, 400, "Неизвестный формат")
			return
		}
		args = append(args, "-f", demuxer)
	}
	args = append(args, "-i", "/proc/self/fd/3", "-map", "0:a:0", "-vn", "-t", "1800", "-ac", "2", "-ar", "44100", "-c:a", "libmp3lame", "-b:a", "192k", "-f", "mp3", "pipe:1")
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.ExtraFiles = []*os.File{f}
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		fail(w, 500, "Decoder unavailable")
		return
	}
	if err = cmd.Start(); err != nil {
		fail(w, 503, "FFmpeg недоступен")
		return
	}
	defer func() { cancel(); cmd.Wait() }()
	buf := make([]byte, 8192)
	n, err := pipe.Read(buf)
	if n == 0 {
		fail(w, 422, "Не удалось декодировать файл")
		return
	}
	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Cache-Control", "no-store")
	if _, e := w.Write(buf[:n]); e != nil {
		return
	}
	if err == nil {
		io.Copy(w, pipe)
	}
}
func (a *App) upload(w http.ResponseWriter, r *http.Request) {
	select {
	case a.uploadSlots <- struct{}{}:
		defer func() { <-a.uploadSlots }()
	default:
		fail(w, 503, "Загрузка занята")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload+65536)
	mr, e := r.MultipartReader()
	if e != nil {
		fail(w, 400, "Ожидается multipart")
		return
	}
	part, e := mr.NextPart()
	if e != nil {
		fail(w, 400, "Нет файла")
		return
	}
	defer part.Close()
	name := part.FileName()
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00") || strings.HasPrefix(name, ".") || len(name) > 240 {
		fail(w, 400, "Некорректное имя")
		return
	}
	source := "uploads"
	dir := r.URL.Query().Get("dir")
	if a.musicWritable {
		source = "music"
	}
	if dir == "" {
		dir = "."
	}
	if !validDirectory(dir) {
		fail(w, 400, "Некорректная папка")
		return
	}
	if source == "uploads" && dir != "." {
		fail(w, 400, "Музыкальный каталог только для чтения")
		return
	}
	root := a.roots[source]
	if !safeDirectory(root, dir) {
		fail(w, 400, "Папка не существует или недоступна")
		return
	}
	name = filepath.ToSlash(filepath.Join(dir, name))
	t, ok := makeTrack(source, name, 1)
	if !ok {
		fail(w, 400, "Неподдерживаемый формат")
		return
	}
	tempName := filepath.Join(dir, ".upload-"+fmt.Sprint(time.Now().UnixNano()))
	temp, err := root.OpenFile(tempName, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		fail(w, 500, "Хранилище недоступно")
		return
	}
	tmp := tempName
	defer root.Remove(tmp)
	limit := int64(maxUpload)
	if t.Mode == "ay" {
		limit = 65536
	}
	n, err := io.Copy(temp, io.LimitReader(part, limit+1))
	closeErr := temp.Close()
	if err != nil || closeErr != nil || n == 0 || n > limit {
		fail(w, 400, "Пустой, слишком большой или повреждённый файл")
		return
	}
	// Serialize publication with scans, then publish without overwriting an existing track.
	a.scanMu.Lock()
	defer a.scanMu.Unlock()
	if err = root.Link(tmp, name); err != nil {
		if errors.Is(err, fs.ErrExist) {
			fail(w, 409, "Файл уже существует")
			return
		}
		fail(w, 500, "Не удалось сохранить файл")
		return
	}
	t, _ = makeTrack(source, name, n)
	a.mu.Lock()
	a.tracks[t.ID] = t
	a.mu.Unlock()
	respond(w, t)
}
func (a *App) playlists(w http.ResponseWriter, r *http.Request) {
	rows, e := a.db.Query("SELECT name,ids FROM playlists ORDER BY name")
	if e != nil {
		fail(w, 500, "Database error")
		return
	}
	defer rows.Close()
	out := []Playlist{}
	for rows.Next() {
		var p Playlist
		var raw string
		if rows.Scan(&p.Name, &raw) != nil || json.Unmarshal([]byte(raw), &p.IDs) != nil {
			fail(w, 500, "Database error")
			return
		}
		out = append(out, p)
	}
	if rows.Err() != nil {
		fail(w, 500, "Database error")
		return
	}
	respond(w, out)
}
func (a *App) savePlaylist(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var p Playlist
	if json.NewDecoder(r.Body).Decode(&p) != nil || strings.TrimSpace(p.Name) == "" || len(p.Name) > 120 || len(p.IDs) > 10000 {
		fail(w, 400, "Некорректный плейлист")
		return
	}
	for _, id := range p.IDs {
		if len(id) != 32 {
			fail(w, 400, "Invalid track ID")
			return
		}
	}
	raw, _ := json.Marshal(p.IDs)
	if _, e := a.db.Exec("INSERT INTO playlists(name,ids) VALUES(?,?) ON CONFLICT(name) DO UPDATE SET ids=excluded.ids", p.Name, string(raw)); e != nil {
		fail(w, 500, "Database error")
		return
	}
	respond(w, p)
}
func (a *App) deletePlaylist(w http.ResponseWriter, r *http.Request) {
	if _, e := a.db.Exec("DELETE FROM playlists WHERE name=?", r.URL.Query().Get("name")); e != nil {
		fail(w, 500, "Database error")
		return
	}
	respond(w, map[string]bool{"ok": true})
}
func guard(next http.Handler, user, pass string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'wasm-unsafe-eval'; style-src 'self'; media-src 'self' blob:; img-src 'self' data:; worker-src 'self' blob:; frame-src 'self'; connect-src 'self'; frame-ancestors 'none'; object-src 'none'; base-uri 'self'")
		// MilkDrop equations require dynamic evaluation; isolate it in an opaque-origin sandbox.
		if r.URL.Path == "/milkdrop.html" {
			w.Header().Del("X-Frame-Options")
			w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self' 'unsafe-eval'; style-src 'self'; img-src data: blob:; connect-src 'none'; frame-ancestors 'self'; base-uri 'none'; form-action 'none'; sandbox allow-scripts")
		}
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			w.Header().Set("Access-Control-Allow-Origin", "null")
		}
		if r.URL.Path == "/healthz" {
			w.Write([]byte("ok"))
			return
		}
		if pass != "" {
			u, p, ok := r.BasicAuth()
			uh, ph := sha256.Sum256([]byte(u)), sha256.Sum256([]byte(p))
			eu, ep := sha256.Sum256([]byte(user)), sha256.Sum256([]byte(pass))
			if !ok || subtle.ConstantTimeCompare(uh[:], eu[:])&subtle.ConstantTimeCompare(ph[:], ep[:]) != 1 {
				w.Header().Set("WWW-Authenticate", `Basic realm="RuneAmp", charset="UTF-8"`)
				w.WriteHeader(401)
				return
			}
		}
		if r.Method != "GET" && r.Method != "HEAD" && r.Header.Get("X-RuneAmp-Request") != "1" {
			fail(w, 403, "Missing request header")
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}
func main() {
	pass := os.Getenv("RUNEAMP_PASSWORD")
	if pass == "" {
		log.Fatal("Set RUNEAMP_PASSWORD for administrator access")
	}
	data := env("RUNEAMP_DATA", "/data")
	uploads := env("RUNEAMP_UPLOADS", "/uploads")
	music := env("RUNEAMP_MUSIC", "/music")
	for _, p := range []string{data, uploads} {
		if e := os.MkdirAll(p, 0700); e != nil {
			log.Fatal(e)
		}
	}
	roots := map[string]*os.Root{}
	for k, p := range map[string]string{"music": music, "uploads": uploads} {
		root, e := os.OpenRoot(p)
		if e != nil {
			log.Fatal(e)
		}
		defer root.Close()
		roots[k] = root
	}
	db, e := sql.Open("sqlite", filepath.Join(data, "runeamp.db"))
	if e != nil {
		log.Fatal(e)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, e = db.Exec("CREATE TABLE IF NOT EXISTS playlists(name TEXT PRIMARY KEY, ids TEXT NOT NULL)"); e != nil {
		log.Fatal(e)
	}
	a := &App{roots: roots, db: db, slots: make(chan struct{}, 2), uploadSlots: make(chan struct{}, 2), musicWritable: os.Getenv("RUNEAMP_MUSIC_WRITABLE") == "1"}
	accounts, err := newAccounts(db, env("RUNEAMP_USER", "admin"), pass)
	if err != nil {
		log.Fatal(err)
	}
	if e = a.scan(); e != nil {
		log.Fatal(e)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/library", a.library)
	mux.HandleFunc("POST /api/rescan", accounts.require("admin", a.rescan))
	mux.HandleFunc("GET /api/media/{id}", a.media)
	mux.HandleFunc("GET /api/stream/{id}", a.stream)
	mux.HandleFunc("POST /api/upload", accounts.require("admin", a.upload))
	mux.HandleFunc("POST /api/import-zip", accounts.require("admin", a.importZIP))
	mux.HandleFunc("GET /api/playlists", a.playlists)
	mux.HandleFunc("PUT /api/playlists", accounts.require("admin", a.savePlaylist))
	mux.HandleFunc("DELETE /api/playlists", accounts.require("admin", a.deletePlaylist))
	mux.HandleFunc("GET /api/me", accounts.me)
	mux.HandleFunc("POST /api/login", func(w http.ResponseWriter, r *http.Request) { accounts.auth(w, r, false) })
	mux.HandleFunc("POST /api/register", func(w http.ResponseWriter, r *http.Request) { accounts.auth(w, r, true) })
	mux.HandleFunc("POST /api/logout", accounts.logout)
	for _, method := range []string{"GET", "PUT", "DELETE"} {
		mux.HandleFunc(method+" /api/personal-playlists", accounts.require("listener", accounts.personal))
	}
	mux.HandleFunc("GET /api/admin/folder", accounts.require("admin", a.folderContents))
	mux.HandleFunc("GET /api/admin/folders", accounts.require("admin", a.folders))
	mux.HandleFunc("POST /api/admin/folders", accounts.require("admin", a.mkdir))
	web := env("RUNEAMP_WEB", "/app/web")
	mux.HandleFunc("GET /playlists", func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, filepath.Join(web, "index.html")) })
	mux.HandleFunc("GET /admin", func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, filepath.Join(web, "index.html")) })
	mux.Handle("/", http.FileServer(http.Dir(web)))
	server := &http.Server{Addr: env("RUNEAMP_LISTEN", ":8080"), Handler: guard(mux, "", ""), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 10 * time.Minute, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32768}
	log.Printf("RuneAmp listening on %s", server.Addr)
	log.Fatal(server.ListenAndServe())
}
