package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setup(t *testing.T) *App {
	t.Helper()
	roots := map[string]*os.Root{}
	for _, k := range []string{"music", "uploads"} {
		p := t.TempDir()
		r, e := os.OpenRoot(p)
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { r.Close() })
		roots[k] = r
	}
	db, e := sql.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { db.Close() })
	if _, e = db.Exec("CREATE TABLE playlists(name TEXT PRIMARY KEY, ids TEXT NOT NULL)"); e != nil {
		t.Fatal(e)
	}
	return &App{roots: roots, tracks: map[string]Track{}, db: db, slots: make(chan struct{}, 2), uploadSlots: make(chan struct{}, 2)}
}
func TestScanRangeAndEscape(t *testing.T) {
	a := setup(t)
	r := a.roots["music"]
	r.Mkdir("album", 0700)
	r.WriteFile("album/test.mp3", []byte("0123456789"), 0600)
	r.WriteFile("ignore.txt", []byte("no"), 0600)
	outside := filepath.Join(t.TempDir(), "secret.mp3")
	os.WriteFile(outside, []byte("secret"), 0600)
	os.Symlink(outside, filepath.Join(r.Name(), "secret.mp3"))
	if e := a.scan(); e != nil {
		t.Fatal(e)
	}
	if len(a.tracks) != 1 {
		t.Fatal(a.tracks)
	}
	id := trackID("music", "album/test.mp3")
	req := httptest.NewRequest("GET", "/api/media/"+id, nil)
	req.SetPathValue("id", id)
	req.Header.Set("Range", "bytes=2-5")
	w := httptest.NewRecorder()
	a.media(w, req)
	if w.Code != 206 || w.Body.String() != "2345" {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	r.Remove("album/test.mp3")
	os.Symlink(outside, filepath.Join(r.Name(), "album/test.mp3"))
	if _, f, e := a.get(id); e == nil {
		f.Close()
		t.Fatal("escaped root")
	}
}
func TestUploadNoOverwrite(t *testing.T) {
	a := setup(t)
	upload := func(name, data string) *httptest.ResponseRecorder {
		var b bytes.Buffer
		m := multipart.NewWriter(&b)
		p, _ := m.CreateFormFile("file", name)
		p.Write([]byte(data))
		m.Close()
		req := httptest.NewRequest("POST", "/api/upload", &b)
		req.Header.Set("Content-Type", m.FormDataContentType())
		w := httptest.NewRecorder()
		a.upload(w, req)
		return w
	}
	if w := upload("track.pt3", "0123456789abcdef"); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if w := upload("track.pt3", "different"); w.Code != 409 {
		t.Fatal(w.Code)
	}
	if w := upload("bad.html", "x"); w.Code != 400 {
		t.Fatal(w.Code)
	}
	b, e := a.roots["uploads"].ReadFile("track.pt3")
	if e != nil || string(b) != "0123456789abcdef" {
		t.Fatal(e, string(b))
	}
}
func TestPlaylists(t *testing.T) {
	a := setup(t)
	id := trackID("music", "test.mp3")
	w := httptest.NewRecorder()
	a.savePlaylist(w, httptest.NewRequest("PUT", "/api/playlists", strings.NewReader(`{"name":"Mix","ids":["`+id+`"]}`)))
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	w = httptest.NewRecorder()
	a.playlists(w, httptest.NewRequest("GET", "/api/playlists", nil))
	var p []Playlist
	if json.Unmarshal(w.Body.Bytes(), &p) != nil || len(p) != 1 || p[0].IDs[0] != id {
		t.Fatal(w.Body.String())
	}
}
func TestGuard(t *testing.T) {
	h := guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }), "admin", "secret")
	for _, x := range []struct {
		method       string
		auth, header bool
		want         int
	}{{"GET", false, false, 401}, {"GET", true, false, 204}, {"POST", true, false, 403}, {"POST", true, true, 204}} {
		r := httptest.NewRequest(x.method, "/api/test", nil)
		if x.auth {
			r.SetBasicAuth("admin", "secret")
		}
		if x.header {
			r.Header.Set("X-RuneAmp-Request", "1")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != x.want {
			t.Fatal(w.Code, x.want)
		}
	}
}
