package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmbeddedPagesWithoutSourceFiles(t *testing.T) {
	t.Chdir(t.TempDir())
	mux := http.NewServeMux()
	registerPageRoutes(mux)
	for _, page := range []string{"login", "register", "groups", "group", "profile"} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", "/"+page, nil))
		if w.Code != 200 || !strings.Contains(w.Body.String(), "<!DOCTYPE html>") {
			t.Fatalf("%s returned %d", page, w.Code)
		}
		if page == "login" || page == "register" {
			if strings.Contains(w.Body.String(), `href="/auth/google"`) || strings.Contains(w.Body.String(), `href="/auth/github"`) {
				t.Fatal("OAuth option still visible")
			}
			if !strings.Contains(w.Body.String(), `action="/auth/`+page+`"`) {
				t.Fatal("missing password form")
			}
		}
	}
}

func TestPasswordRegistrationAndLogin(t *testing.T) {
	db, err := NewDatabase(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	h := NewAuthHandler(db, &AuthConfig{BaseURL: "https://bible.soh.re"})
	post := func(handler http.HandlerFunc, path string, values url.Values) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", path, strings.NewReader(values.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		handler(w, r)
		return w
	}
	form := url.Values{"email": {"form@example.test"}, "first_name": {"Form"}, "last_name": {"Test"}, "password": {"Password-test-42!"}}
	w := post(h.handleRegister, "/auth/register", form)
	if w.Code != 303 || w.Header().Get("Location") != "/" {
		t.Fatalf("registration failed: %d %s", w.Code, w.Header().Get("Location"))
	}
	if len(w.Result().Cookies()) == 0 || !w.Result().Cookies()[0].Secure {
		t.Fatal("missing secure session cookie")
	}
	w = post(h.handleRegister, "/auth/register", form)
	if !strings.HasPrefix(w.Header().Get("Location"), "/register?error=") {
		t.Fatal("duplicate registration did not return to form")
	}
	w = post(h.handleLogin, "/auth/login", url.Values{"identifier": {"form@example.test"}, "password": {"Password-test-42!"}})
	if w.Code != 303 || w.Header().Get("Location") != "/" {
		t.Fatal("password sign-in failed")
	}
	w = post(h.handleLogin, "/auth/login", url.Values{"identifier": {"form@example.test"}, "password": {"wrong"}})
	if !strings.HasPrefix(w.Header().Get("Location"), "/login?error=") {
		t.Fatal("invalid password did not return to form")
	}
}
