package main

import "testing"

func TestBrowserAYFormats(t *testing.T) {
	for _, ext := range []string{"pt1", "pt2", "pt3", "stp", "asc", "as0", "psc", "sqt", "stc", "psg", "ym", "vtx"} {
		t.Run(ext, func(t *testing.T) {
			limit := aySizeLimit(ext)
			track, ok := makeTrack("music", "track."+ext, limit)
			if !ok || track.Mode != "ay" {
				t.Fatalf("not browser decoded: %+v", track)
			}
			if _, ok = makeTrack("music", "track."+ext, limit+1); ok {
				t.Fatal("oversize accepted")
			}
		})
	}
}
