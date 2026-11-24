package main

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestAccountsIsolation(t *testing.T) {
	a := setup(t)
	ac, e := newAccounts(a.db, "admin", "admin-secret")
	if e != nil {
		t.Fatal(e)
	}
	call := func(h http.HandlerFunc, method, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/api/test", strings.NewReader(body))
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		h(w, r)
		return w
	}
	reg := func(name string) *http.Cookie {
		w := call(func(w http.ResponseWriter, r *http.Request) { ac.auth(w, r, true) }, "POST", `{"name":"`+name+`","password":"simple123"}`, nil)
		if w.Code != 200 {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
		c := w.Result().Cookies()[0]
		if !c.HttpOnly || c.SameSite != http.SameSiteLaxMode {
			t.Fatal("unsafe cookie")
		}
		return c
	}
	alice, bob := reg("alice"), reg("bob")
	if w := call(ac.require("admin", a.rescan), "POST", "", nil); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := call(ac.require("admin", a.rescan), "POST", "", alice); w.Code != 403 {
		t.Fatal(w.Code)
	}
	body := `{"name":"Private","ids":["01234567890123456789012345678901"]}`
	if w := call(ac.personal, "PUT", body, alice); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w := call(ac.personal, "GET", "", bob); w.Body.String() != "[]\n" {
		t.Fatal(w.Body.String())
	}
	if w := call(ac.personal, "GET", "", alice); !strings.Contains(w.Body.String(), "Private") {
		t.Fatal(w.Body.String())
	}
	w := call(func(w http.ResponseWriter, r *http.Request) { ac.auth(w, r, false) }, "POST", `{"name":"admin","password":"admin-secret"}`, nil)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	admin := w.Result().Cookies()[0]
	if w = call(ac.require("admin", a.rescan), "POST", "", admin); w.Code != 200 {
		t.Fatal(w.Code)
	}
	w = call(ac.logout, "POST", "", alice)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w = call(ac.personal, "GET", "", alice); w.Code != 401 {
		t.Fatal(w.Code)
	}
	var salt, hash []byte
	if e = a.db.QueryRow("SELECT salt,hash FROM listeners WHERE name='alice'").Scan(&salt, &hash); e != nil || len(salt) != 16 || len(hash) != 32 || bytes.Equal(hash, []byte("simple123")) {
		t.Fatal("password storage", e)
	}
	w = call(func(w http.ResponseWriter, r *http.Request) { ac.auth(w, r, false) }, "POST", `{"name":"alice","password":"wrong"}`, nil)
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
}
func TestPublicAndAdminUpload(t *testing.T) {
	a := setup(t)
	a.musicWritable = true
	r := a.roots["music"]
	r.Mkdir("ZX", 0755)
	upload := func(dir string) *httptest.ResponseRecorder {
		var b bytes.Buffer
		m := multipart.NewWriter(&b)
		p, _ := m.CreateFormFile("file", "track.pt3")
		p.Write([]byte("0123456789abcdef"))
		m.Close()
		req := httptest.NewRequest("POST", "/api/upload?dir="+dir, &b)
		req.Header.Set("Content-Type", m.FormDataContentType())
		w := httptest.NewRecorder()
		a.upload(w, req)
		return w
	}
	if w := upload("ZX"); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if _, e := r.ReadFile("ZX/track.pt3"); e != nil {
		t.Fatal(e)
	}
	if w := upload("ZX"); w.Code != 409 {
		t.Fatal(w.Code)
	}
	if w := upload("../"); w.Code != 400 {
		t.Fatal(w.Code)
	}
	outside := t.TempDir()
	os.Symlink(outside, r.Name()+"/escape")
	if w := upload("escape"); w.Code != 400 {
		t.Fatal(w.Code)
	}
	w := httptest.NewRecorder()
	a.mkdir(w, httptest.NewRequest("POST", "/api/admin/folders", strings.NewReader(`{"path":"ZX/New"}`)))
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	w = httptest.NewRecorder()
	a.folders(w, httptest.NewRequest("GET", "/api/admin/folders", nil))
	var data struct {
		Writable bool
		Folders  []string
	}
	if json.Unmarshal(w.Body.Bytes(), &data) != nil || !data.Writable || len(data.Folders) != 3 {
		t.Fatal(w.Body.String())
	}
}
