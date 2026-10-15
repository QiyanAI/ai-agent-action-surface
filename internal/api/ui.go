package api

import (
	"embed"
	"io/fs"
	"net/http"
)

// The local console: static HTML/CSS/JS embedded in the binary, served by the
// same process that owns state. No external CDN, no build step, no framework
// (book §06, §16).
//
// Hardening:
//   - a Content-Security-Policy that forbids inline script and external
//     origins, so a rendering mistake cannot become script injection;
//   - all dynamic rendering in the page scripts goes through textContent
//     helpers (see app.js expectText contract), never innerHTML with data;
//   - state-changing calls still require the session cookie AND the CSRF
//     header; the bootstrap token only mints a session.

//go:embed uiassets
var uiAssets embed.FS

func (s *Server) uiRoutes() {
	sub, err := fs.Sub(uiAssets, "uiassets")
	if err != nil {
		panic("api: embedded ui assets missing: " + err.Error())
	}
	// StripPrefix hands the embedded FS clean paths ("/", "/app.css"); the
	// file server then serves index.html for the directory root itself.
	fileServer := http.StripPrefix("/ui", http.FileServer(http.FS(sub)))
	s.mux.HandleFunc("GET /ui", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ui/", http.StatusFound)
	})
	s.mux.HandleFunc("GET /ui/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy",
			"default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		fileServer.ServeHTTP(w, r)
	})
}
