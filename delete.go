package main

import (
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func deleteAllHandler(w http.ResponseWriter, r *http.Request) {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		writeJSON(w, http.StatusOK, messageResponse{Message: "Not Authorized!"})
		return
	}

	// strings.Split on a single space, then index 1: "Bearer" with no token
	// yields "", and repeated spaces yield empty elements. Both fail the check.
	parts := strings.Split(auth, " ")
	token := ""
	if len(parts) > 1 {
		token = parts[1]
	}

	deleteToken, err := os.ReadFile(".delete-token")
	if err != nil {
		log.Printf("read delete token: %v", err)
		http.Error(w, "Delete token is not configured", http.StatusServiceUnavailable)
		return
	}
	configuredToken := strings.TrimSpace(string(deleteToken))
	if configuredToken == "" {
		log.Println("delete token is not configured")
		http.Error(w, "Delete token is not configured", http.StatusServiceUnavailable)
		return
	}

	if token != configuredToken {
		// Preserved quirk: the original never wrote a response on a bad token,
		// so the request hangs until the client goes away. Do not write here.
		<-r.Context().Done()
		return
	}

	deleteSongs()
	writeJSON(w, http.StatusOK, messageResponse{Message: "Authorized"})
}

// deleteSongs removes only files whose names pass isValidFile. Directories
// (including non-audio files) are left alone. Walk errors are logged and stop
// the walk, but never change the HTTP response.
func deleteSongs() {
	_ = filepath.WalkDir(uploadRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			log.Println(err)
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !isValidFile(d.Name()) {
			return nil
		}
		if removeErr := os.Remove(p); removeErr != nil {
			log.Println(removeErr)
			return removeErr
		}
		return nil
	})
}
