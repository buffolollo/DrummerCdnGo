package main

import (
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Package-level constant on purpose; the original did not read this from env.
const deleteToken = "aNs54If4sdfG4e68f4eGsa65f4Ese65Rf4se65fe4es65f4es65f4"

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

	if token != deleteToken {
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
