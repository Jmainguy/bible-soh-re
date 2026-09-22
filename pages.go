package main

import "net/http"

// Clean URLs must use embedded assets: release images do not contain source files.
func registerPageRoutes(mux *http.ServeMux) {
	for _, page := range []string{"login", "register", "groups", "group", "profile"} {
		mux.HandleFunc("/"+page, func(w http.ResponseWriter, r *http.Request) {
			http.ServeFileFS(w, r, staticFiles, "static/"+page+".html")
		})
	}
}
