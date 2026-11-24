package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestZIPRecoverDamagedNestedAndEntries(t *testing.T) {
	a := setup(t)
	a.musicWritable = true
	root := a.roots["music"]
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for _, name := range []string{"ok-before.mod", "empty/broken.mod", "same.mod", "ok-after.mod", "bad-crc.zip"} {
		f, e := w.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
		if e != nil {
			t.Fatal(e)
		}
		if _, e = f.Write([]byte("payload")); e != nil {
			t.Fatal(e)
		}
	}
	if e := w.Close(); e != nil {
		t.Fatal(e)
	}
	corrupt := b.Bytes()
	r, e := zip.NewReader(bytes.NewReader(corrupt), int64(len(corrupt)))
	if e != nil {
		t.Fatal(e)
	}
	for _, f := range r.File {
		if f.Name == "empty/broken.mod" || f.Name == "same.mod" || f.Name == "bad-crc.zip" {
			off, e := f.DataOffset()
			if e != nil {
				t.Fatal(e)
			}
			corrupt[off] ^= 0xff
		}
	}
	inner := zipData(t, zipEntry{"good.mod", []byte("nested"), 0}, zipEntry{"bad.zip", []byte(strings.Repeat("x", 40)), 0}, zipEntry{"last.mod", []byte("last"), 0})
	outer := zipData(t, zipEntry{"MOD.LIB/A/first.mod", []byte("first"), 0}, zipEntry{"MOD.LIB/E/echoing2.zip", []byte(strings.Repeat("x", 40)), 0}, zipEntry{"MOD.LIB/E/crc.zip", corrupt, 0}, zipEntry{"MOD.LIB/E/recover.zip", inner, 0}, zipEntry{"MOD.LIB/E/same.mod", []byte("valid replacement"), 0}, zipEntry{"MOD.LIB/E/last.mod", []byte("duplicate"), 0}, zipEntry{"MOD.LIB/Z/end.mod", []byte("end"), 0})
	res := zipRequest(t, a, ".", "MOD.LIB.zip", outer)
	if res.Code != 200 {
		t.Fatal(res.Code, res.Body.String())
	}
	var report zipReport
	if e := json.Unmarshal(res.Body.Bytes(), &report); e != nil || report.Tracks != 7 || report.Archives != 3 || report.BadArchives != 3 || report.BadFiles != 2 || report.Duplicates != 1 || len(report.Warnings) != 5 {
		t.Fatal(res.Body.String(), e)
	}
	for p, want := range map[string]string{"A/first.mod": "first", "E/ok-before.mod": "payload", "E/ok-after.mod": "payload", "E/good.mod": "nested", "E/last.mod": "last", "E/same.mod": "valid replacement", "Z/end.mod": "end"} {
		data, e := root.ReadFile("MOD.LIB/MOD.LIB/" + p)
		if e != nil || string(data) != want {
			t.Fatal(p, string(data), e)
		}
	}
	if _, e := root.Lstat("MOD.LIB/MOD.LIB/E/empty"); !os.IsNotExist(e) {
		t.Fatal("partial file or empty directory retained", e)
	}
	if len(a.tracks) != 7 {
		t.Fatal(len(a.tracks))
	}
	entries, e := os.ReadDir(root.Name())
	if e != nil || len(entries) != 1 || entries[0].Name() != "MOD.LIB" {
		t.Fatal(entries, e)
	}
	if _, e := os.Stat(filepath.Join(root.Name(), "MOD.LIB/MOD.LIB/E/echoing2.zip")); !os.IsNotExist(e) {
		t.Fatal("retained bad archive", e)
	}
}
