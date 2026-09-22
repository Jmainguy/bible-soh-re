package main

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// Clean URLs must use embedded assets: release images do not contain source files.
func registerPageRoutes(mux *http.ServeMux) {
	for _, page := range []string{"login", "register", "groups", "group", "profile"} {
		mux.HandleFunc("/"+page, func(w http.ResponseWriter, r *http.Request) {
			http.ServeFileFS(w, r, staticFiles, "static/"+page+".html")
		})
	}
}

// Keep a real 404 status while giving readers a useful route back.
func newStaticHandler(assets fs.FS) http.Handler {
	files := http.FileServer(http.FS(assets))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" {
			name = "."
		}
		info, err := fs.Stat(assets, name)
		if err == nil && info.IsDir() {
			_, err = fs.Stat(assets, path.Join(name, "index.html"))
		}
		if err == nil {
			files.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			respondError(w, "Not found", http.StatusNotFound)
			return
		}
		page, err := staticFiles.ReadFile("static/404.html")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusNotFound)
		if r.Method != http.MethodHead {
			_, _ = w.Write(page)
		}
	})
}
