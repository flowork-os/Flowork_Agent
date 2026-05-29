// Package httpx — small shared HTTP helpers used by every internal package.
//
// Kept dependency-free so every other package can import it without
// pulling app-wide state. Two pieces: WriteJSON sets the standard headers
// and indents responses for human inspection; NoCache wraps a handler so
// the embedded UI never gets cached by the browser during development.

package httpx

import (
	"encoding/json"
	"net/http"
)

// WriteJSON marshals body to indented JSON, sets the content type, and
// disables caching. Any encoding error is silently dropped — callers
// already checked their payload before calling.
func WriteJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(body)
}

// NoCache wraps a handler so every response carries Cache-Control: no-store.
// Iterating on the embedded UI without this turns into a fight with the
// browser cache.
func NoCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
