package main

import (
	"archive/zip"
	"context"
	"encoding/binary"
	"os"
	"testing"
)

func TestZIPCentralDirectoryAndLegacyName(t *testing.T) {
	a := setup(t)
	a.musicWritable = true
	data := zipData(t, zipEntry{"Caf\x82/song.mp3", []byte("music"), 0})
	w := zipRequest(t, a, ".", "Legacy.zip", data)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if _, e := a.roots["music"].ReadFile("Legacy/Café/song.mp3"); e != nil {
		t.Fatal(e)
	}
	bad := append([]byte(nil), data...)
	binary.LittleEndian.PutUint16(bad[len(bad)-22+8:], 0)
	binary.LittleEndian.PutUint16(bad[len(bad)-22+10:], 0)
	w = zipRequest(t, a, ".", "Forged.zip", bad)
	if w.Code != 422 {
		t.Fatal(w.Code, w.Body.String())
	}
}
func TestZIPExpandedBudget(t *testing.T) {
	a := setup(t)
	r := a.roots["music"]
	work := a.roots["uploads"]
	data := zipData(t, zipEntry{"song.mp3", []byte("0123456789"), 0})
	f, e := work.OpenFile("source.zip", os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	f.Write(data)
	z := zipImport{ctx: context.Background(), root: r, work: work, budget: 5}
	if e = z.extract(f, int64(len(data)), ".", 0); e == nil {
		t.Fatal("budget ignored")
	}
	if z.report.Tracks != 0 {
		t.Fatal(z.report)
	}
	ar, e := zip.NewReader(f, int64(len(data)))
	if e != nil || len(ar.File) != 1 {
		t.Fatal(e)
	}
}
