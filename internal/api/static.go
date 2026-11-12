package api

import (
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
)

// newWebHandler serves the built web client from webDir. Files are served as
// they are; any other GET request gets index.html so the single-page app can
// handle the route. The client is served through an fs.FS rooted at webDir, so
// a request cannot reach files outside it.
func newWebHandler(webDir string) http.Handler {
	fsys := os.DirFS(webDir)
	files := http.FileServerFS(fsys)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}

		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name != "" && fs.ValidPath(name) {
			if info, err := fs.Stat(fsys, name); err == nil && !info.IsDir() {
				files.ServeHTTP(w, r)
				return
			}
		}

		if _, err := fs.Stat(fsys, "index.html"); err != nil {
			writeError(w, http.StatusServiceUnavailable, "The web client has not been built. Run make build or npm run build.")
			return
		}
		http.ServeFileFS(w, r, fsys, "index.html")
	})
}
