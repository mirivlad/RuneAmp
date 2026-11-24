package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type zipEntry struct {
	name string
	data []byte
	mode os.FileMode
}

func zipData(t *testing.T, entries ...zipEntry) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for _, entry := range entries {
		h := &zip.FileHeader{Name: entry.name, Method: zip.Deflate}
		if entry.mode != 0 {
			h.SetMode(entry.mode)
		}
		f, e := w.CreateHeader(h)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = f.Write(entry.data); e != nil {
			t.Fatal(e)
		}
	}
	if e := w.Close(); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func zipRequest(t *testing.T, a *App, dir, name string, data []byte) *httptest.ResponseRecorder {
	t.Helper()
	var b bytes.Buffer
	m := multipart.NewWriter(&b)
	f, e := m.CreateFormFile("file", name)
	if e != nil {
		t.Fatal(e)
	}
	f.Write(data)
	m.Close()
	r := httptest.NewRequest("POST", "/api/import-zip?dir="+dir, &b)
	r.Header.Set("Content-Type", m.FormDataContentType())
	w := httptest.NewRecorder()
	a.importZIP(w, r)
	return w
}
func TestNestedZIPImport(t *testing.T) {
	a := setup(t)
	a.musicWritable = true
	root := a.roots["music"]
	root.Mkdir("ZX", 0755)
	inner := zipData(t, zipEntry{"same.pt3", []byte("first version"), 0}, zipEntry{"Deep/song.PT2", []byte("pt2"), 0}, zipEntry{"notes.txt", []byte("ignore"), 0})
	other := zipData(t, zipEntry{"same.pt3", []byte("second version"), 0}, zipEntry{"Deep/song.PT2", []byte("duplicate"), 0}, zipEntry{"Deep/other.stp", []byte("stp"), 0})
	outer := zipData(t, zipEntry{"Pack/A.zip", inner, 0}, zipEntry{"Pack/B.ZIP", other, 0}, zipEntry{"Else/same.pt3", []byte("different folder"), 0}, zipEntry{"Pack/same.pt3", []byte("third version"), 0}, zipEntry{"plain.mp3", []byte("mp3"), 0}, zipEntry{"../escape.mp3", []byte("bad"), 0}, zipEntry{"C:\\evil.mp3", []byte("bad"), 0}, zipEntry{"Pack/link.mp3", []byte("/etc/passwd"), os.ModeSymlink | 0777}, zipEntry{"cover.jpg", []byte("image"), 0})
	w := zipRequest(t, a, "ZX", "Collection.ZIP", outer)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var report zipReport
	if e := json.Unmarshal(w.Body.Bytes(), &report); e != nil || report.Path != "ZX/Collection" || report.Archives != 3 || report.Tracks != 5 || report.Duplicates != 3 || report.Skipped != 5 {
		t.Fatal(w.Body.String(), e)
	}
	for p, want := range map[string]string{"Pack/same.pt3": "first version", "Pack/Deep/song.PT2": "pt2", "Pack/Deep/other.stp": "stp", "Else/same.pt3": "different folder", "plain.mp3": "mp3"} {
		b, e := root.ReadFile("ZX/Collection/" + p)
		if e != nil || string(b) != want {
			t.Fatal(p, string(b), e)
		}
	}
	if len(a.tracks) != 5 {
		t.Fatal(len(a.tracks))
	}
	if e := a.scan(); e != nil || len(a.tracks) != 5 {
		t.Fatal(e, len(a.tracks))
	}
	for _, p := range []string{"ZX/Collection/Pack/A.zip", "ZX/Collection/Pack/A", "ZX/Collection/cover.jpg", "ZX/escape.mp3", "escape.mp3", "ZX/Collection/Pack/link.mp3"} {
		if _, e := root.Lstat(p); e == nil {
			t.Fatal("unexpected", p)
		}
	}
	w = zipRequest(t, a, "ZX", "Collection.ZIP", outer)
	if w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
	dirs, _ := os.ReadDir(filepath.Join(root.Name(), "ZX"))
	if len(dirs) != 1 || dirs[0].Name() != "Collection" {
		t.Fatal(dirs)
	}
}
func TestZIPRollbackAndLimits(t *testing.T) {
	a := setup(t)
	a.musicWritable = true
	root := a.roots["music"]
	for _, c := range []struct {
		name   string
		data   []byte
		status int
	}{{"Broken.zip", zipData(t, zipEntry{"bad.zip", []byte("not zip"), 0}), 422}, {"Empty.zip", zipData(t, zipEntry{"a.txt", []byte("no"), 0}), 422}, {"Bad.zip", []byte("not zip"), 422}} {
		w := zipRequest(t, a, ".", c.name, c.data)
		if w.Code != c.status {
			t.Fatal(w.Code, w.Body.String())
		}
		if _, e := root.Lstat(strings.TrimSuffix(c.name, ".zip")); e == nil {
			t.Fatal("partial import")
		}
	}
	nested := zipData(t, zipEntry{"music.mp3", []byte("ok"), 0})
	for i := 0; i <= maxZIPDepth; i++ {
		nested = zipData(t, zipEntry{"inner.zip", nested, 0})
	}
	if w := zipRequest(t, a, ".", "Deep.zip", zipData(t, zipEntry{"ok.mp3", []byte("ok"), 0}, zipEntry{"deep.zip", nested, 0})); w.Code != 422 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := zipRequest(t, a, "../", "Escape.zip", nested); w.Code != 400 {
		t.Fatal(w.Code)
	}
	d, _ := os.ReadDir(root.Name())
	if len(d) != 0 || len(a.tracks) != 0 {
		t.Fatal(d, a.tracks)
	}
}
func TestZIPReadOnlyAndAuth(t *testing.T) {
	a := setup(t)
	data := zipData(t, zipEntry{"test.mp3", []byte("music"), 0})
	w := zipRequest(t, a, ".", "Album.zip", data)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if _, e := a.roots["uploads"].ReadFile("Album/test.mp3"); e != nil {
		t.Fatal(e)
	}
	if _, e := a.roots["music"].Lstat("Album"); e == nil {
		t.Fatal("music modified")
	}
	ac, e := newAccounts(a.db, "admin", "secret")
	if e != nil {
		t.Fatal(e)
	}
	w = httptest.NewRecorder()
	ac.require("admin", a.importZIP)(w, httptest.NewRequest(http.MethodPost, "/api/import-zip", nil))
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
}
