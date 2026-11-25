package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func callV030(h http.HandlerFunc, method, url, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, url, strings.NewReader(body))
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	h(w, r)
	return w
}
func TestAppendConcurrentAndRoles(t *testing.T) {
	a := setup(t)
	a.db.SetMaxOpenConns(1)
	ac, e := newAccounts(a.db, "admin", "admin-secret")
	if e != nil {
		t.Fatal(e)
	}
	reg := func(name, pw string, register bool) *http.Cookie {
		w := callV030(func(w http.ResponseWriter, r *http.Request) { ac.auth(w, r, register) }, "POST", "/", `{"name":"`+name+`","password":"`+pw+`"}`, nil)
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		return w.Result().Cookies()[0]
	}
	alice, bob, admin := reg("alice", "simple123", true), reg("bob", "simple123", true), reg("admin", "admin-secret", false)
	ids := []string{}
	for _, name := range []string{"a.mp3", "b.mp3", "c.mp3"} {
		tr, _ := makeTrack("music", name, 1)
		a.tracks[tr.ID] = tr
		ids = append(ids, tr.ID)
	}
	send := func(h http.HandlerFunc, c *http.Cookie, ids []string, create bool) *httptest.ResponseRecorder {
		b, _ := json.Marshal(map[string]any{"name": "Mix", "ids": ids, "create": create})
		return callV030(h, "POST", "/", string(b), c)
	}
	pub, own := a.appendPlaylist(ac, true), a.appendPlaylist(ac, false)
	if w := send(pub, nil, ids, true); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := send(pub, alice, ids, true); w.Code != 403 {
		t.Fatal(w.Code)
	}
	if w := send(own, alice, ids[:1], true); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w := send(own, bob, ids, false); w.Code != 404 {
		t.Fatal(w.Code)
	}
	if w := send(pub, admin, ids[:1], true); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var wg sync.WaitGroup
	for _, id := range ids[1:] {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			if w := send(pub, admin, []string{ids[0], id, id}, false); w.Code != 200 {
				t.Error(w.Body.String())
			}
		}(id)
	}
	wg.Wait()
	var raw string
	if e = a.db.QueryRow("SELECT ids FROM playlists WHERE name='Mix'").Scan(&raw); e != nil {
		t.Fatal(e)
	}
	var got []string
	json.Unmarshal([]byte(raw), &got)
	if len(got) != 3 || got[0] != ids[0] || got[1] == got[2] {
		t.Fatal(got)
	}
}
func TestMovePreservesIDsAfterScanAndRestart(t *testing.T) {
	a := setup(t)
	a.musicWritable = true
	r := a.roots["music"]
	r.MkdirAll("ZX/Old", 0755)
	r.WriteFile("ZX/Old/song.mp3", []byte("music"), 0644)
	if e := a.initIDs(); e != nil {
		t.Fatal(e)
	}
	if e := a.scan(); e != nil {
		t.Fatal(e)
	}
	id := trackID("music", "ZX/Old/song.mp3")
	w := callV030(a.moveEntry, "POST", "/", `{"source":"music","path":"ZX/Old","destination":"ZX/New"}`, nil)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if e := a.initIDs(); e != nil {
		t.Fatal(e)
	}
	if e := a.scan(); e != nil {
		t.Fatal(e)
	}
	if a.tracks[id].Path != "ZX/New/song.mp3" {
		t.Fatal(a.tracks)
	}
	r.Mkdir("ZX/Old", 0755)
	r.WriteFile("ZX/Old/song.mp3", []byte("new music"), 0644)
	if e := a.scan(); e != nil {
		t.Fatal(e)
	}
	if len(a.tracks) != 2 || a.tracks[id].Path != "ZX/New/song.mp3" {
		t.Fatal(a.tracks)
	}
	if w = callV030(a.moveEntry, "POST", "/", `{"source":"music","path":"ZX/New","destination":"ZX/Old"}`, nil); w.Code != 409 {
		t.Fatal(w.Body.String())
	}
	if w = callV030(a.moveEntry, "POST", "/", `{"source":"music","path":"ZX/New","destination":"../escape"}`, nil); w.Code != 400 {
		t.Fatal(w.Code)
	}
}
func TestSharedSkinLifecycle(t *testing.T) {
	a := setup(t)
	if e := a.initSkins(); e != nil {
		t.Fatal(e)
	}
	bmp := make([]byte, 54+276*3*116)
	copy(bmp, "BM")
	binary.LittleEndian.PutUint32(bmp[2:], uint32(len(bmp)))
	binary.LittleEndian.PutUint32(bmp[10:], 54)
	binary.LittleEndian.PutUint32(bmp[14:], 40)
	binary.LittleEndian.PutUint32(bmp[18:], 275)
	binary.LittleEndian.PutUint32(bmp[22:], 116)
	binary.LittleEndian.PutUint16(bmp[26:], 1)
	binary.LittleEndian.PutUint16(bmp[28:], 24)
	archive := zipData(t, zipEntry{"MAIN.BMP", bmp, 0})
	var b bytes.Buffer
	m := multipart.NewWriter(&b)
	p, _ := m.CreateFormFile("file", "Classic.wsz")
	p.Write(archive)
	m.Close()
	req := httptest.NewRequest("POST", "/", &b)
	req.Header.Set("Content-Type", m.FormDataContentType())
	w := httptest.NewRecorder()
	a.skinUpload(w, req)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var s skinInfo
	json.Unmarshal(w.Body.Bytes(), &s)
	call := func(h http.HandlerFunc, method, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/", strings.NewReader(body))
		r.SetPathValue("id", s.ID)
		w := httptest.NewRecorder()
		h(w, r)
		return w
	}
	if w = call(a.skinUpdate, "PUT", `{"visible":true,"default":true}`); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w = call(a.skinList(false), "GET", ""); !strings.Contains(w.Body.String(), `"default":true`) {
		t.Fatal(w.Body.String())
	}
	if w = call(a.skinDownload(false), "GET", ""); !bytes.Equal(w.Body.Bytes(), archive) {
		t.Fatal(w.Code)
	}
	if w = call(a.skinUpdate, "PUT", `{"visible":false,"default":false}`); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w = call(a.skinDownload(false), "GET", ""); w.Code != 404 {
		t.Fatal(w.Code)
	}
	if w = call(a.skinDownload(true), "GET", ""); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w = call(a.skinDelete, "DELETE", ""); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w = call(a.skinDownload(true), "GET", ""); w.Code != 404 {
		t.Fatal(w.Code)
	}
}
