package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeleteMusic(t *testing.T) {
	a := setup(t)
	a.musicWritable = true
	r := a.roots["music"]
	r.MkdirAll("ZX/Pack", 0755)
	r.Mkdir("ZX/Pack2", 0755)
	for _, p := range []string{"ZX/Pack/one.pt3", "ZX/Pack/two.mp3", "ZX/Pack/notes.txt", "ZX/Pack2/keep.mp3"} {
		if e := r.WriteFile(p, []byte("music"), 0644); e != nil {
			t.Fatal(e)
		}
	}
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "keep.mp3"), []byte("keep"), 0600)
	os.Symlink(outside, filepath.Join(r.Name(), "ZX/Pack/link"))
	if e := a.scan(); e != nil {
		t.Fatal(e)
	}
	call := func(source, path string, directory bool) *httptest.ResponseRecorder {
		b, _ := json.Marshal(deleteEntryRequest{source, path, directory})
		w := httptest.NewRecorder()
		a.deleteEntry(w, httptest.NewRequest("DELETE", "/api/admin/entry", strings.NewReader(string(b))))
		return w
	}
	for _, p := range []string{".", "../outside", "/tmp", "ZX/Pack/link/keep.mp3"} {
		if w := call("music", p, false); w.Code != 400 {
			t.Fatal(p, w.Code, w.Body.String())
		}
	}
	if w := call("music", "ZX/Pack/link", true); w.Code != 409 {
		t.Fatal(w.Code)
	}
	if w := call("music", "ZX/Pack", false); w.Code != 409 {
		t.Fatal(w.Code)
	}
	a.musicWritable = false
	if w := call("music", "ZX/Pack/one.pt3", false); w.Code != 403 {
		t.Fatal(w.Code)
	}
	a.musicWritable = true
	if w := call("music", "ZX/Pack/one.pt3", false); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if _, ok := a.tracks[trackID("music", "ZX/Pack/one.pt3")]; ok {
		t.Fatal("stale index")
	}
	if w := call("music", "ZX/Pack", true); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if _, e := r.Lstat("ZX/Pack"); !os.IsNotExist(e) {
		t.Fatal(e)
	}
	if len(a.tracks) != 1 {
		t.Fatal(a.tracks)
	}
	if _, e := r.ReadFile("ZX/Pack2/keep.mp3"); e != nil {
		t.Fatal(e)
	}
	if _, e := os.ReadFile(filepath.Join(outside, "keep.mp3")); e != nil {
		t.Fatal(e)
	}
	if w := call("music", "ZX/Pack", true); w.Code != 404 {
		t.Fatal(w.Code)
	}
	ac, e := newAccounts(a.db, "admin", "secret")
	if e != nil {
		t.Fatal(e)
	}
	w := httptest.NewRecorder()
	ac.require("admin", a.deleteEntry)(w, httptest.NewRequest("DELETE", "/api/admin/entry", nil))
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
}
