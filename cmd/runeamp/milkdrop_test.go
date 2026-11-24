package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMilkdropIsolation(t *testing.T) {
	h := guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }), "", "")
	for _, path := range []string{"/", "/admin", "/api/library", "/assets/main.js", "/milkdrop.html"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		c := w.Header().Get("Content-Security-Policy")
		if path == "/milkdrop.html" {
			for _, rule := range []string{"sandbox allow-scripts", "connect-src 'none'", "script-src 'self' 'unsafe-eval'", "frame-ancestors 'self'"} {
				if !strings.Contains(c, rule) {
					t.Fatal(rule, c)
				}
			}
			if strings.Contains(c, "allow-same-origin") || w.Header().Get("X-Frame-Options") != "" {
				t.Fatal("unsafe or blocked sandbox")
			}
		} else if strings.Contains(c, "'unsafe-eval'") || w.Header().Get("X-Frame-Options") != "DENY" {
			t.Fatal(path, c)
		}
		if path == "/assets/main.js" {
			if w.Header().Get("Access-Control-Allow-Origin") != "null" {
				t.Fatal("missing static CORS")
			}
		} else if w.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Fatal("CORS beyond static assets", path)
		}
	}
}
