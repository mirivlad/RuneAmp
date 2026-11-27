package main

import (
	"bytes"
	"encoding/binary"
	"os"
)

// Detect the PT3 subtype without changing the stable path-based track ID.
func classifyAY(t Track, root *os.Root, name string) Track {
	if t.Format != "pt3" || t.Size < 203 || t.Size > aySizeLimit("pt3") {
		return t
	}
	f, err := root.Open(name)
	if err != nil {
		return t
	}
	defer f.Close()
	tail := make([]byte, 16)
	if _, err = f.ReadAt(tail, t.Size-16); err == nil && bytes.Equal(tail[12:], []byte("02TS")) && bytes.Equal(tail[:4], []byte("PT3!")) && bytes.Equal(tail[6:10], []byte("PT3!")) {
		a, b := int64(binary.LittleEndian.Uint16(tail[4:])), int64(binary.LittleEndian.Uint16(tail[10:]))
		if a >= 203 && b >= 203 && a+b+16 == t.Size {
			t.Format = "ts"
			return t
		}
	}
	if t.Size > 65536 {
		return t
	}
	head := make([]byte, 203)
	if _, err = f.ReadAt(head, 0); err == nil && head[98] != 32 && head[98] > 0 && head[98] <= 86 && head[100] > 0 && head[201] != 255 {
		marker := int(head[98])
		p := int(head[201])
		ptr := int(binary.LittleEndian.Uint16(head[103:]))
		q := marker*3 - 3 - p
		if p%3 == 0 && q >= 0 && q <= 254 && q%3 == 0 && int64(ptr+2*max(p, q)+6) <= t.Size {
			t.Format = "ts"
		}
	}
	return t
}
