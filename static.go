package main

import (
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// uploadFileSystem is the custom http.FileSystem used by the static handler.
// It deliberately hides anything that resolves to a directory (express.static
// has no auto-index here) and any dotfile/dot-directory. Returning
// os.ErrNotExist makes net/http answer 404.
type uploadFileSystem struct {
	root string
}

func (fsys uploadFileSystem) Open(name string) (http.File, error) {
	clean := path.Clean("/" + name)
	if clean == "/" {
		return nil, os.ErrNotExist
	}

	for _, component := range strings.Split(strings.TrimPrefix(clean, "/"), "/") {
		if component == "" || strings.HasPrefix(component, ".") {
			return nil, os.ErrNotExist
		}
	}

	// path.Clean has already removed any ".." components; joining with the
	// fixed root therefore cannot escape it.
	filePath := filepath.Join(fsys.root, filepath.FromSlash(clean))
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}

	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if info.IsDir() {
		_ = f.Close()
		return nil, os.ErrNotExist
	}
	return f, nil
}

// staticHandler serves a single file using http.ServeContent, which provides
// ranges, Last-Modified, and If-Modified-Since handling. The weak ETag format
// mirrors the stat ETags produced by express/send.
func staticHandler(fsys http.FileSystem) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Path

		f, err := fsys.Open(name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()

		info, err := f.Stat()
		if err != nil || info.IsDir() {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Cache-Control", "public, max-age=0")
		w.Header().Set("ETag", weakETag(info))
		if ct := contentTypeFor(name); ct != "" {
			w.Header().Set("Content-Type", ct)
		}

		http.ServeContent(w, r, info.Name(), info.ModTime(), f)
	})
}

func weakETag(info os.FileInfo) string {
	return fmt.Sprintf(`W/"%x-%x"`, info.Size(), info.ModTime().UnixMilli())
}

var contentTypeByExtension = map[string]string{
	".mp3":  "audio/mpeg",
	".wav":  "audio/wav",
	".flac": "audio/flac",
	".m4a":  "audio/mp4",
}

// Empty means "let http.ServeContent use the extension table / sniffing".
func contentTypeFor(name string) string {
	return contentTypeByExtension[strings.ToLower(filepath.Ext(name))]
}
