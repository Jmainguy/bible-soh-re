package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestChapterErrorsAreJSON(t *testing.T) {
	oldNames, oldTranslations := translationNames, translations
	t.Cleanup(func() { translationNames, translations = oldNames, oldTranslations })
	translations = map[string]*Translation{}
	for _, tc := range []struct {
		query  string
		names  []string
		status int
	}{
		{"?book=Genesis&chapter=1", nil, http.StatusServiceUnavailable},
		{"?book=Genesis&chapter=1&translation=nasb", nil, http.StatusBadRequest},
		{"?book=Genesis&chapter=invalid&translation=nasb", []string{"nasb"}, http.StatusBadRequest},
	} {
		translationNames = tc.names
		w := httptest.NewRecorder()
		handleChapter(w, httptest.NewRequest("GET", "/api/chapter"+tc.query, nil))
		if w.Code != tc.status {
			t.Fatalf("%s: got %d", tc.query, w.Code)
		}
		var body map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body["error"] == "" {
			t.Fatalf("expected JSON error, got %s", w.Body.String())
		}
	}
}
