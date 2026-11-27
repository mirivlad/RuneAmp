package main

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Identity struct {
	Name string `json:"name"`
	Role string `json:"role"`
}
type Accounts struct {
	db              *sql.DB
	admin, password string
	secure          bool
	mu              sync.Mutex
	window          time.Time
	attempts        int
	slots           chan struct{}
}

func newAccounts(db *sql.DB, admin, pass string) (*Accounts, error) {
	_, e := db.Exec(`CREATE TABLE IF NOT EXISTS listeners(name TEXT PRIMARY KEY COLLATE NOCASE, salt BLOB NOT NULL, hash BLOB NOT NULL);
 CREATE TABLE IF NOT EXISTS sessions(token TEXT PRIMARY KEY, name TEXT NOT NULL, role TEXT NOT NULL, expires INTEGER NOT NULL);
 CREATE TABLE IF NOT EXISTS personal_playlists(owner TEXT NOT NULL,name TEXT NOT NULL,ids TEXT NOT NULL,PRIMARY KEY(owner,name));`)
	return &Accounts{db: db, admin: admin, password: pass, secure: os.Getenv("RUNEAMP_SECURE_COOKIE") == "1", slots: make(chan struct{}, 2)}, e
}
func hashToken(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
func (a *Accounts) who(r *http.Request) Identity {
	c, e := r.Cookie("runeamp_session")
	if e != nil || len(c.Value) != 64 {
		return Identity{"", "guest"}
	}
	var i Identity
	e = a.db.QueryRow("SELECT name,role FROM sessions WHERE token=? AND expires>?", hashToken(c.Value), time.Now().Unix()).Scan(&i.Name, &i.Role)
	if e != nil {
		return Identity{"", "guest"}
	}
	if i.Role == "admin" && (a.password == "" || i.Name != a.admin) {
		return Identity{"", "guest"}
	}
	return i
}
func (a *Accounts) cookie(w http.ResponseWriter, r *http.Request, token string, age int) {
	http.SetCookie(w, &http.Cookie{Name: "runeamp_session", Value: token, Path: "/", HttpOnly: true, Secure: a.secure || r.TLS != nil, SameSite: http.SameSiteLaxMode, MaxAge: age})
}
func (a *Accounts) me(w http.ResponseWriter, r *http.Request) { respond(w, a.who(r)) }

var validName = regexp.MustCompile(`^[a-zA-Z0-9_.-]{3,32}$`)

func key(password string, salt []byte) []byte {
	b, _ := pbkdf2.Key(sha256.New, password, salt, 600000, 32)
	return b
}
func (a *Accounts) allowAttempt() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if time.Since(a.window) > time.Minute {
		a.window = time.Now()
		a.attempts = 0
	}
	if a.attempts >= 30 {
		return false
	}
	a.attempts++
	return true
}
func (a *Accounts) auth(w http.ResponseWriter, r *http.Request, register bool) {
	if !a.allowAttempt() {
		fail(w, 429, "Слишком много попыток, попробуйте через минуту")
		return
	}
	select {
	case a.slots <- struct{}{}:
		defer func() { <-a.slots }()
	default:
		fail(w, 429, "Попробуйте чуть позже")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var p struct{ Name, Password string }
	if json.NewDecoder(r.Body).Decode(&p) != nil || len(p.Password) < 1 || len(p.Password) > 256 || len(p.Name) > 32 {
		fail(w, 400, "Некорректные данные")
		return
	}
	p.Name = strings.TrimSpace(p.Name)
	id := Identity{strings.ToLower(p.Name), "listener"}
	if register {
		if !validName.MatchString(p.Name) || len(p.Password) < 6 || strings.EqualFold(p.Name, a.admin) {
			fail(w, 400, "Логин 3–32 символа (латиница, цифры, ._-), пароль от 6 символов")
			return
		}
		salt := make([]byte, 16)
		if _, e := rand.Read(salt); e != nil {
			fail(w, 500, "Ошибка регистрации")
			return
		}
		if _, e := a.db.Exec("INSERT INTO listeners(name,salt,hash) VALUES(?,?,?)", id.Name, salt, key(p.Password, salt)); e != nil {
			fail(w, 409, "Логин занят или регистрация недоступна")
			return
		}
	} else if strings.EqualFold(p.Name, a.admin) && a.password != "" {
		got, want := sha256.Sum256([]byte(p.Password)), sha256.Sum256([]byte(a.password))
		if subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
			fail(w, 401, "Неверный логин или пароль")
			return
		}
		id = Identity{a.admin, "admin"}
	} else {
		salt := make([]byte, 16)
		expected := make([]byte, 32)
		e := a.db.QueryRow("SELECT salt,hash FROM listeners WHERE name=?", id.Name).Scan(&salt, &expected)
		got := key(p.Password, salt)
		if subtle.ConstantTimeCompare(got, expected) != 1 || e != nil {
			fail(w, 401, "Неверный логин или пароль")
			return
		}
	}
	a.revoke(r)
	raw := make([]byte, 32)
	if _, e := rand.Read(raw); e != nil {
		fail(w, 500, "Ошибка входа")
		return
	}
	token := hex.EncodeToString(raw)
	age := 30 * 86400
	if id.Role == "admin" {
		age = 12 * 3600
	}
	a.db.Exec("DELETE FROM sessions WHERE expires<=?", time.Now().Unix())
	if _, e := a.db.Exec("INSERT INTO sessions(token,name,role,expires) VALUES(?,?,?,?)", hashToken(token), id.Name, id.Role, time.Now().Unix()+int64(age)); e != nil {
		fail(w, 500, "Ошибка входа")
		return
	}
	a.cookie(w, r, token, age)
	respond(w, id)
}
func (a *Accounts) revoke(r *http.Request) {
	if c, e := r.Cookie("runeamp_session"); e == nil && len(c.Value) == 64 {
		a.db.Exec("DELETE FROM sessions WHERE token=?", hashToken(c.Value))
	}
}
func (a *Accounts) logout(w http.ResponseWriter, r *http.Request) {
	a.revoke(r)
	a.cookie(w, r, "", -1)
	respond(w, Identity{"", "guest"})
}
func (a *Accounts) require(role string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := a.who(r)
		if id.Role == "guest" {
			fail(w, 401, "Войдите в аккаунт")
			return
		}
		if role == "admin" && id.Role != "admin" {
			fail(w, 403, "Требуются права администратора")
			return
		}
		h(w, r)
	}
}
func (a *Accounts) personal(w http.ResponseWriter, r *http.Request) {
	id := a.who(r)
	if id.Role == "guest" {
		fail(w, 401, "Войдите в аккаунт")
		return
	}
	switch r.Method {
	case "GET":
		rows, e := a.db.Query("SELECT name,ids FROM personal_playlists WHERE owner=? ORDER BY name", id.Name)
		if e != nil {
			fail(w, 500, "Ошибка БД")
			return
		}
		defer rows.Close()
		out := []Playlist{}
		for rows.Next() {
			var p Playlist
			var raw string
			if rows.Scan(&p.Name, &raw) != nil || json.Unmarshal([]byte(raw), &p.IDs) != nil {
				fail(w, 500, "Ошибка БД")
				return
			}
			out = append(out, p)
		}
		if rows.Err() != nil {
			fail(w, 500, "Ошибка БД")
			return
		}
		respond(w, out)
	case "PUT":
		r.Body = http.MaxBytesReader(w, r.Body, maxPlaylistBody)
		var p Playlist
		if json.NewDecoder(r.Body).Decode(&p) != nil || strings.TrimSpace(p.Name) == "" || len(p.Name) > 120 {
			fail(w, 400, "Некорректный плейлист")
			return
		}
		for _, v := range p.IDs {
			if len(v) != 32 {
				fail(w, 400, "Некорректный трек")
				return
			}
		}
		raw, _ := json.Marshal(p.IDs)
		_, e := a.db.Exec("INSERT INTO personal_playlists(owner,name,ids) VALUES(?,?,?) ON CONFLICT(owner,name) DO UPDATE SET ids=excluded.ids", id.Name, p.Name, string(raw))
		if e != nil {
			fail(w, 500, "Ошибка БД")
			return
		}
		respond(w, p)
	case "DELETE":
		_, e := a.db.Exec("DELETE FROM personal_playlists WHERE owner=? AND name=?", id.Name, r.URL.Query().Get("name"))
		if e != nil {
			fail(w, 500, "Ошибка БД")
			return
		}
		respond(w, map[string]bool{"ok": true})
	}
}
