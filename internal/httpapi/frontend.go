package httpapi

import (
	"bytes"
	"crypto/rand"
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:assets
var assets embed.FS

// nonceMarker is the placeholder in index.html that each application shell response
// replaces with its own nonce.
const nonceMarker = "__CSP_NONCE__"

// contentSecurityPolicy allows scripts and styles from the console origin only. The
// application shell passes a per-response nonce for the style elements that dialog scroll
// locking injects. Inline scripts and inline style attributes stay blocked; React applies
// style properties through the CSS object model, which the policy does not restrict.
func contentSecurityPolicy(styleNonce string) string {
	styles := "'self'"
	if styleNonce != "" {
		styles += " 'nonce-" + styleNonce + "'"
	}
	return "default-src 'self'; script-src 'self'; style-src " + styles + "; img-src 'self' data:; " +
		"font-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'"
}

func embeddedAssets() fs.FS {
	static, _ := fs.Sub(assets, "assets")
	return static
}

// frontend serves static assets. The root, index.html, and paths without a file extension
// return the application shell for client-side routes.
func frontend(static fs.FS) http.HandlerFunc {
	files := http.FileServerFS(static)
	shell, shellErr := fs.ReadFile(static, "index.html")
	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name != "" && name != "index.html" {
			if _, err := fs.Stat(static, name); err == nil {
				files.ServeHTTP(w, r)
				return
			}
			if strings.Contains(name, ".") {
				http.NotFound(w, r)
				return
			}
		}
		if shellErr != nil {
			http.Error(w, "Frontend unavailable. Run make build or use make dev.", http.StatusServiceUnavailable)
			return
		}
		// Every response is no-store (see secure), so a page never reuses a nonce from an
		// earlier response.
		nonce := rand.Text()
		w.Header().Set("Content-Security-Policy", contentSecurityPolicy(nonce))
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(bytes.ReplaceAll(shell, []byte(nonceMarker), []byte(nonce)))
	}
}
