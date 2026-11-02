package main

import (
	"io/fs"
	"log"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
)

// Valid extensions are compared in lowercase form, including the leading dot.
var validExtensions = map[string]struct{}{
	".flac": {},
	".wav":  {},
	".mp3":  {},
	".m4a":  {},
}

func isValidFile(name string) bool {
	_, ok := validExtensions[strings.ToLower(filepath.Ext(name))]
	return ok
}

// song field order is significant: name, url, lastModified.
type song struct {
	Name         string `json:"name"`
	URL          string `json:"url"`
	LastModified int64  `json:"lastModified"`
}

type songsResponse struct {
	Message string `json:"message"`
	Songs   []song `json:"songs"`
}

func listFilesHandler(w http.ResponseWriter, r *http.Request) {
	songs := collectSongs()
	if len(songs) == 0 {
		// The original omits the songs key entirely when nothing was found.
		writeJSON(w, http.StatusOK, messageResponse{Message: "error"})
		return
	}
	writeJSON(w, http.StatusOK, songsResponse{Message: "success", Songs: songs})
}

func collectSongs() []song {
	var songs []song

	err := filepath.WalkDir(uploadRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Directories are always traversed, regardless of their names.
		if d.IsDir() {
			return nil
		}
		if !isValidFile(d.Name()) {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}
		songs = append(songs, song{
			Name:         d.Name(),
			URL:          listCDNBase + filepath.ToSlash(p),
			LastModified: info.ModTime().UnixMilli(),
		})
		return nil
	})
	if err != nil {
		// A missing uploads directory (or any other walk failure) yields an
		// empty list rather than an error response.
		log.Println(err)
		return nil
	}

	// Stable sort keeps the os walk order for equal timestamps.
	sort.SliceStable(songs, func(i, j int) bool {
		return songs[i].LastModified > songs[j].LastModified
	})
	return songs
}
