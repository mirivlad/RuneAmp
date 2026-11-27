package main

import (
	"encoding/binary"
	"testing"
)

func TestTurboSoundClassification(t *testing.T) {
	a := setup(t)
	root := a.roots["music"]
	single := make([]byte, 600)
	single[98] = 32
	single[100] = 1
	single[201] = 0
	single[202] = 255
	native := append([]byte{}, single...)
	native[98] = 48
	native[201] = 141
	binary.LittleEndian.PutUint16(native[103:], 220)
	pair := append(append([]byte{}, single...), single...)
	tail := make([]byte, 16)
	copy(tail, "PT3!")
	copy(tail[6:], "PT3!")
	copy(tail[12:], "02TS")
	binary.LittleEndian.PutUint16(tail[4:], 600)
	binary.LittleEndian.PutUint16(tail[10:], 600)
	pair = append(pair, tail...)
	for _, v := range []struct {
		name string
		data []byte
		want string
	}{{"single.pt3", single, "pt3"}, {"native.pt3", native, "ts"}, {"pair.pt3", pair, "ts"}, {"pair.ts", pair, "ts"}} {
		if e := root.WriteFile(v.name, v.data, 0644); e != nil {
			t.Fatal(e)
		}
	}
	if e := a.scan(); e != nil {
		t.Fatal(e)
	}
	for name, want := range map[string]string{"single.pt3": "pt3", "native.pt3": "ts", "pair.pt3": "ts", "pair.ts": "ts"} {
		id := trackID("music", name)
		if a.tracks[id].Format != want {
			t.Fatalf("%s: %+v", name, a.tracks[id])
		}
	}
}
