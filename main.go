package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

const (
	// Both uploads and listings are published under the same working CDN domain.
	uploadCDNBase = "https://drummercdn.buffolollo.it/"
	listCDNBase   = "https://drummercdn.buffolollo.it/"

	// Uploads are stored relative to the process working directory.
	uploadRoot = "uploads"

	corsOrigin = "https://drummer.buffolollo.it"
)

func main() {
	// Keep log output free of the default timestamp prefix so the startup line
	// is byte-for-byte what the original process printed.
	log.SetFlags(0)

	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, messageResponse{Message: "Ok"})
	})
	// The original Express route accepts both /upload and /upload/.
	mux.HandleFunc("POST /upload", uploadHandler)
	mux.HandleFunc("POST /upload/{$}", uploadHandler)
	mux.HandleFunc("GET /files", listFilesHandler)
	mux.HandleFunc("GET /deleteall", deleteAllHandler)

	// express.static emulation. StripPrefix removes /uploads, leaving a rooted
	// path for the custom http.FileSystem passed to staticHandler.
	mux.Handle("GET /uploads/", http.StripPrefix("/uploads", staticHandler(uploadFileSystem{root: uploadRoot})))

	// Catch-all: net/http's method-aware mux would answer 405 for a known path
	// with the wrong method. Express answers 404 for anything unmatched, so use
	// a methodless subtree pattern as the fallback.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})

	handler := recoverMiddleware(corsMiddleware(mux))

	fmt.Println("CDN => Now listening on http://localhost:1211")
	if err := http.ListenAndServe(":1211", handler); err != nil {
		log.Println(err)
	}
}

// corsMiddleware reproduces the Node cors package configured with a fixed
// origin and credentials enabled.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Access-Control-Allow-Origin", corsOrigin)
		h.Set("Access-Control-Allow-Credentials", "true")

		vary := "Origin"
		if r.Method == http.MethodOptions {
			h.Set("Access-Control-Allow-Methods", "GET,HEAD,PUT,PATCH,POST,DELETE")
			if requested := r.Header.Get("Access-Control-Request-Headers"); requested != "" {
				h.Set("Access-Control-Allow-Headers", requested)
				vary += ", Access-Control-Request-Headers"
			}
			h.Set("Vary", vary)
			// The cors package ends every preflight itself (204 + CL: 0).
			// net/http drops a canonical Content-Length from a 204 response,
			// so use a non-canonical key that its suppression list misses.
			// Header names are case-insensitive on the wire.
			h.Set("Content-Length", "0")
			h["Content-length"] = []string{"0"}
			w.WriteHeader(http.StatusNoContent)
			return
		}

		h.Set("Vary", vary)
		next.ServeHTTP(w, r)
	})
}

// recoverMiddleware keeps a handler panic from killing the process.
func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic: %v", rec)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type messageResponse struct {
	Message string `json:"message"`
}

// writeJSON uses SetEscapeHTML(false) to match JSON.stringify, which does not
// turn &, < and > into \u escapes. The encoder's trailing newline is removed
// because Express sends the bare JSON text.
func writeJSON(w http.ResponseWriter, status int, v any) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		log.Println(err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	body := bytes.TrimSuffix(buf.Bytes(), []byte{'\n'})
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
