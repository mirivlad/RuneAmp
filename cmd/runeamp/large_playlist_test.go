package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func TestLargePlaylistAppend(t *testing.T) {
	a := setup(t)
	a.db.SetMaxOpenConns(1)
	ac, e := newAccounts(a.db, "admin", "admin-secret")
	if e != nil {
		t.Fatal(e)
	}
	w := callV030(func(w http.ResponseWriter, r *http.Request) { ac.auth(w, r, false) }, "POST", "/", `{"name":"admin","password":"admin-secret"}`, nil)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	cookie := w.Result().Cookies()[0]
	ids := make([]string, 100000)
	for i := range ids {
		tr, _ := makeTrack("music", fmt.Sprintf("ZX/%06d.pt3", i), 1000)
		a.tracks[tr.ID] = tr
		ids[i] = tr.ID
	}
	h := a.appendPlaylist(ac, true)
	for i := 0; i < len(ids); i += 10000 {
		batch := append([]string{}, ids[i:i+10000]...)
		batch = append(batch, batch[0], "missing")
		body, _ := json.Marshal(map[string]any{"name": "Archive", "ids": batch, "create": i == 0})
		w = callV030(h, "POST", "/", string(body), cookie)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var result struct{ Added, Skipped, Unavailable int }
		if e = json.Unmarshal(w.Body.Bytes(), &result); e != nil || result.Added != 10000 || result.Skipped != 1 || result.Unavailable != 1 {
			t.Fatal(result, e)
		}
	}
	var raw string
	if e = a.db.QueryRow("SELECT ids FROM playlists WHERE name='Archive'").Scan(&raw); e != nil {
		t.Fatal(e)
	}
	var got []string
	if e = json.Unmarshal([]byte(raw), &got); e != nil || len(got) != len(ids) {
		t.Fatal(len(got), e)
	}
	for i, id := range ids {
		if got[i] != id {
			t.Fatal("order mismatch", i)
		}
	}
}
