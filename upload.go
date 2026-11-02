package main

import (
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	maxUploadSize = 100 * 1024 * 1024 // 100 MiB, same as Multer's limit.
	songFieldName = "song"
)

// Multer's diskStorage sanitises each individual character, without
// collapsing runs of them.
var invalidFilenameChars = regexp.MustCompile(`[<>:"/\\|?*\s]`)

type uploadResponse struct {
	Message     string `json:"message"`
	Description string `json:"description"`
	URL         string `json:"url"`
}

type multerErrorDetail struct {
	Name    string `json:"name"`
	Message string `json:"message"`
	Code    string `json:"code"`
	Field   string `json:"field"`
}

type multerErrorResponse struct {
	Message     string            `json:"message"`
	Description string            `json:"description"`
	Error       multerErrorDetail `json:"error"`
}

type unknownErrorResponse struct {
	Message     string   `json:"message"`
	Description string   `json:"description"`
	Error       struct{} `json:"error"`
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	// Streaming parse on purpose: ParseMultipartForm would buffer to disk in a
	// way that does not match Multer's single-file diskStorage behavior.
	mr, err := r.MultipartReader()
	if err != nil {
		if errors.Is(err, http.ErrNotMultipart) {
			// Preserved quirk: a non-multipart request is still a success, but
			// with the literal URL suffix "undefined".
			writeUploadSuccess(w, "")
			return
		}
		// E.g. multipart/form-data without a boundary: Multer surfaces this as
		// a generic (non-Multer) error.
		log.Println(err)
		writeUnknownError(w)
		return
	}

	var createdFiles []string
	cleanup := func() {
		for _, p := range createdFiles {
			_ = os.Remove(p)
		}
	}

	songSeen := false
	storedRelPath := ""

	for {
		part, partErr := mr.NextPart()
		if partErr == io.EOF {
			break
		}
		if partErr != nil {
			log.Println(partErr)
			cleanup()
			writeUnknownError(w)
			return
		}

		field := part.FormName()
		filename := part.FileName()

		// A part without a filename is an ordinary form field; Multer accepts
		// those regardless of their name.
		if filename == "" {
			_ = part.Close()
			continue
		}

		// Any file part other than the single "song" file is unexpected. This
		// also catches a second "song" file.
		if field != songFieldName || songSeen {
			_ = part.Close()
			cleanup()
			writeMulterError(w, "LIMIT_UNEXPECTED_FILE", "Unexpected field", field)
			return
		}
		songSeen = true

		// File filter: the ORIGINAL filename's lowercased extension must be
		// valid, otherwise this is a generic 500 (not a Multer error).
		if !isValidFile(filename) {
			_ = part.Close()
			cleanup()
			log.Printf("invalid upload extension: %q", filename)
			writeUnknownError(w)
			return
		}

		// The sub-folder keeps the original case of the extension. It is never
		// created here: a missing folder makes os.Create fail with a 500.
		format := filename[strings.LastIndex(filename, ".")+1:]
		relativePath := filepath.Join(uploadRoot, format, sanitizeFilename(filename))

		f, createErr := os.Create(relativePath)
		if createErr != nil {
			_ = part.Close()
			cleanup()
			log.Println(createErr)
			writeUnknownError(w)
			return
		}
		createdFiles = append(createdFiles, relativePath)

		// Read one byte past the limit so an oversized file can be detected
		// without buffering it in memory.
		written, copyErr := io.Copy(f, io.LimitReader(part, maxUploadSize+1))
		closeErr := f.Close()
		_ = part.Close()

		if copyErr != nil {
			log.Println(copyErr)
			cleanup()
			writeUnknownError(w)
			return
		}
		if closeErr != nil {
			log.Println(closeErr)
			cleanup()
			writeUnknownError(w)
			return
		}
		if written > maxUploadSize {
			cleanup()
			writeMulterError(w, "LIMIT_FILE_SIZE", "File too large", songFieldName)
			return
		}

		storedRelPath = filepath.ToSlash(relativePath)
	}

	if !songSeen {
		// Preserved quirk: no file at all produces the success JSON with
		// "undefined" as the URL.
		writeUploadSuccess(w, "")
		return
	}
	writeUploadSuccess(w, storedRelPath)
}

func sanitizeFilename(name string) string {
	return invalidFilenameChars.ReplaceAllString(name, "_")
}

func writeUploadSuccess(w http.ResponseWriter, relativePath string) {
	url := uploadCDNBase + "undefined"
	if relativePath != "" {
		url = uploadCDNBase + relativePath
	}
	writeJSON(w, http.StatusOK, uploadResponse{
		Message:     "success",
		Description: "Song uploaded successfully!",
		URL:         url,
	})
}

func writeMulterError(w http.ResponseWriter, code, message, field string) {
	writeJSON(w, http.StatusBadRequest, multerErrorResponse{
		Message:     "error",
		Description: "Multer error",
		Error: multerErrorDetail{
			Name:    "MulterError",
			Message: message,
			Code:    code,
			Field:   field,
		},
	})
}

func writeUnknownError(w http.ResponseWriter) {
	// A JS Error serialises to {}, hence the empty object here.
	writeJSON(w, http.StatusInternalServerError, unknownErrorResponse{
		Message:     "error",
		Description: "Unknown error",
	})
}
