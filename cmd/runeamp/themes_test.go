package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testThemeManifest() themeManifest {
	p := map[string]string{}
	for _, k := range paletteKeys {
		p[k] = "#123456"
	}
	return themeManifest{Version: 1, Name: "Example", Dark: p, Light: p, Radius: 3, Typography: "mono", Artwork: "industrial"}
}
func themeZIP(t *testing.T, m themeManifest, extra map[string][]byte) []byte {
	t.Helper()
	b := new(bytes.Buffer)
	z := zip.NewWriter(b)
	raw, _ := json.Marshal(m)
	w, e := z.Create("theme.json")
	if e != nil {
		t.Fatal(e)
	}
	w.Write(raw)
	for n, d := range extra {
		w, e = z.Create(n)
		if e != nil {
			t.Fatal(e)
		}
		w.Write(d)
	}
	if e = z.Close(); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func TestServiceThemeValidation(t *testing.T) {
	m := testThemeManifest()
	if _, _, e := readTheme(themeZIP(t, m, nil)); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"../evil.js", "style.css", "page.html", "evil.svg"} {
		if _, _, e := readTheme(themeZIP(t, m, map[string][]byte{name: []byte("x")})); e == nil {
			t.Fatalf("accepted %s", name)
		}
	}
	m.Dark["bg"] = "red;display:none"
	if _, _, e := readTheme(themeZIP(t, m, nil)); e == nil {
		t.Fatal("accepted CSS injection")
	}
	m = testThemeManifest()
	m.Images = map[string]string{"hero": "art.png"}
	b := new(bytes.Buffer)
	png.Encode(b, image.NewRGBA(image.Rect(0, 0, 8, 8)))
	_, art, e := readTheme(themeZIP(t, m, map[string][]byte{"art.png": b.Bytes()}))
	if e != nil || !strings.HasPrefix(art["hero"], "data:image/png;base64,") {
		t.Fatal(e, art)
	}
	if _, _, e = readTheme(themeZIP(t, m, map[string][]byte{"art.png": []byte("not image")})); e == nil {
		t.Fatal("accepted invalid image")
	}
}
func TestServiceThemeLifecycle(t *testing.T) {
	a := setup(t)
	if e := a.initThemes(); e != nil {
		t.Fatal(e)
	}
	ac, e := newAccounts(a.db, "admin", "admin-secret")
	if e != nil {
		t.Fatal(e)
	}
	login := callV030(func(w http.ResponseWriter, r *http.Request) { ac.auth(w, r, false) }, "POST", "/", `{"name":"admin","password":"admin-secret"}`, nil)
	if login.Code != 200 {
		t.Fatal(login.Body.String())
	}
	admin := login.Result().Cookies()[0]
	req := func(method, url, body string, h http.HandlerFunc, c *http.Cookie) *httptest.ResponseRecorder {
		return callV030(h, method, url, body, c)
	}
	active := func() string {
		r := req("GET", "/api/theme", "", a.activeTheme, nil)
		var v themeConfig
		json.Unmarshal(r.Body.Bytes(), &v)
		return v.ID
	}
	if active() != "default" {
		t.Fatal("initial theme")
	}
	if r := req("PUT", "/api/admin/themes/active", `{"id":"default"}`, ac.require("admin", a.themeApply), nil); r.Code != 401 && r.Code != 403 {
		t.Fatal("guest can mutate", r.Code)
	}
	b := new(bytes.Buffer)
	mp := multipart.NewWriter(b)
	p, _ := mp.CreateFormFile("file", "example.zip")
	p.Write(themeZIP(t, testThemeManifest(), nil))
	mp.Close()
	r := httptest.NewRequest("POST", "/api/admin/themes", b)
	r.Header.Set("Content-Type", mp.FormDataContentType())
	r.AddCookie(admin)
	w := httptest.NewRecorder()
	ac.require("admin", a.themeUpload)(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var info themeInfo
	json.Unmarshal(w.Body.Bytes(), &info)
	if active() != "default" {
		t.Fatal("upload auto-activated")
	}
	apply := func(id string) {
		body, _ := json.Marshal(map[string]string{"id": id})
		r := req("PUT", "/api/admin/themes/active", string(body), ac.require("admin", a.themeApply), admin)
		if r.Code != 200 {
			t.Fatal(r.Body.String())
		}
	}
	apply(info.ID)
	if active() != info.ID {
		t.Fatal("apply")
	}
	if e := a.initThemes(); e != nil || active() != info.ID {
		t.Fatal("state lost on init", e)
	}
	apply("default")
	if active() != "default" {
		t.Fatal("reset")
	}
	apply(info.ID)
	d := httptest.NewRequest("DELETE", "/api/admin/themes/"+info.ID, nil)
	d.SetPathValue("id", info.ID)
	d.AddCookie(admin)
	dw := httptest.NewRecorder()
	ac.require("admin", a.themeDelete)(dw, d)
	if dw.Code != 200 || active() != "default" {
		t.Fatal("active deletion did not reset")
	}
}
