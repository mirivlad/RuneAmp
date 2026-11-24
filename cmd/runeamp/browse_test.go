package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestBrowseActualFolder(t *testing.T) {
	a := setup(t)
	root := a.roots["music"]
	root.Mkdir("ZX", 0755)
	root.Mkdir("ZX/Empty", 0755)
	root.WriteFile("ZX/song.pt3", []byte("0123456789abcdef"), 0600)
	root.WriteFile("ZX/note.txt", []byte("ignore"), 0600)
	os.Symlink(t.TempDir(), filepath.Join(root.Name(), "ZX/escape"))
	if e := a.scan(); e != nil {
		t.Fatal(e)
	}
	root.WriteFile("ZX/new.mp3", []byte("new"), 0600)
	read := func(dir string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		a.folderContents(w, httptest.NewRequest("GET", "/api/admin/folder?dir="+dir, nil))
		return w
	}
	w := read("ZX")
	var data struct {
		Entries         []FolderEntry
		Tracks, Folders int
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &data) != nil || data.Tracks != 2 || data.Folders != 1 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, e := range data.Entries {
		if e.Name == "new.mp3" && e.Indexed {
			t.Fatal("unexpected index")
		}
		if e.Name == "song.pt3" && !e.Indexed {
			t.Fatal("missing index")
		}
	}
	if read("ZX/Empty").Code != 200 || read("../").Code != 400 || read("ZX/escape").Code != 400 {
		t.Fatal("folder validation")
	}
}
