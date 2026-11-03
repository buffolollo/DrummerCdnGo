package main

import (
	"errors"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// errInvalidFilename marks a filename that cannot safely be deleted.
var errInvalidFilename = errors.New("invalid file name")

// readDeleteToken loads and validates the token stored in .delete-token.
func readDeleteToken() (string, error) {
	data, err := os.ReadFile(".delete-token")
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", errors.New("delete token is not configured")
	}
	return token, nil
}

// deleteFileHandler deletes a single stored file. It accepts the file name and
// the delete token as query parameters:
//
//	GET /files/delete?filename=track.mp3&token=...
//
// A bare file name is matched against every sub-folder under uploadRoot, while
// a relative path (for example "mp3/track.mp3") targets one file directly.
func deleteFileHandler(w http.ResponseWriter, r *http.Request) {
	filename := strings.TrimSpace(r.URL.Query().Get("filename"))
	if filename == "" {
		writeJSON(w, http.StatusBadRequest, messageResponse{Message: "File name is required"})
		return
	}

	configuredToken, err := readDeleteToken()
	if err != nil {
		log.Printf("read delete token: %v", err)
		http.Error(w, "Delete token is not configured", http.StatusServiceUnavailable)
		return
	}
	if r.URL.Query().Get("token") != configuredToken {
		writeJSON(w, http.StatusUnauthorized, messageResponse{Message: "Not Authorized!"})
		return
	}

	deleted, err := deleteStoredFile(filename)
	switch {
	case errors.Is(err, errInvalidFilename):
		writeJSON(w, http.StatusBadRequest, messageResponse{Message: "Invalid file name"})
	case err != nil:
		log.Printf("delete file %q: %v", filename, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	case !deleted:
		writeJSON(w, http.StatusNotFound, messageResponse{Message: "File not found"})
	default:
		writeJSON(w, http.StatusOK, messageResponse{Message: "success"})
	}
}

// deleteStoredFile removes the requested file and reports whether anything was
// deleted. Paths that escape uploadRoot are rejected.
func deleteStoredFile(name string) (bool, error) {
	clean := filepath.Clean(filepath.FromSlash(name))
	if filepath.IsAbs(clean) {
		return false, errInvalidFilename
	}

	parts := strings.Split(clean, string(filepath.Separator))
	for _, part := range parts {
		if part == ".." {
			return false, errInvalidFilename
		}
	}

	// A bare name is looked up in every format sub-folder. This mirrors the
	// listing endpoint, which returns the base name without its directory.
	if len(parts) == 1 {
		return deleteFilesByName(clean)
	}

	target := filepath.Join(uploadRoot, clean)
	rootAbs, err := filepath.Abs(uploadRoot)
	if err != nil {
		return false, err
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return false, err
	}
	if targetAbs != rootAbs && !strings.HasPrefix(targetAbs, rootAbs+string(filepath.Separator)) {
		return false, errInvalidFilename
	}

	info, err := os.Lstat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if !info.Mode().IsRegular() || !isValidFile(info.Name()) {
		return false, errInvalidFilename
	}
	if err := os.Remove(target); err != nil {
		return false, err
	}
	return true, nil
}

// deleteFilesByName removes every regular audio file whose base name matches.
func deleteFilesByName(name string) (bool, error) {
	if !isValidFile(name) {
		return false, errInvalidFilename
	}

	deleted := false
	err := filepath.WalkDir(uploadRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != name || !d.Type().IsRegular() {
			return nil
		}
		if removeErr := os.Remove(p); removeErr != nil {
			return removeErr
		}
		deleted = true
		return nil
	})
	if err != nil && os.IsNotExist(err) {
		return false, nil
	}
	return deleted, err
}

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

	configuredToken, err := readDeleteToken()
	if err != nil {
		log.Printf("read delete token: %v", err)
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
