# Audio CDN (Go reimplementation)

A drop-in Go reimplementation of the Node.js/Express/TypeScript audio CDN used by
<https://drummer.buffolollo.it>. It uses only the Go standard library and listens
on port **1211**.

## Run

Requires Go 1.22 or newer.

```sh
go run .
```

On start it prints:

```
CDN => Now listening on http://localhost:1211
```

Uploads live under `uploads/` relative to the current working directory. The
destination folders are **not** created automatically (matching Multer's
behavior), so create them before uploading:

```sh
mkdir -p uploads/mp3 uploads/wav uploads/flac uploads/m4a
```

## Endpoints

```sh
# Health check
curl http://localhost:1211/

# Upload a song (multipart field must be exactly "song")
curl -F "song=@track.mp3" http://localhost:1211/upload

# List all stored songs
curl http://localhost:1211/upload/files

# Delete a single stored song (the token is in .delete-token)
curl "http://localhost:1211/files/delete?filename=track.mp3&token=YOUR_DELETE_TOKEN"

# Delete all stored songs (the token is in .delete-token)
curl -H "Authorization: Bearer YOUR_DELETE_TOKEN" \
  http://localhost:1211/upload/deleteall

# Static audio file (supports HTTP Range for streaming)
curl -H "Range: bytes=0-1023" http://localhost:1211/uploads/mp3/track.mp3
```

## Intentionally preserved quirks

- Upload and listing responses both use `https://drummercdn.buffolollo.it/`.
- A non-multipart request, or a multipart request with no `song` file, returns
  HTTP 200 success with the URL `https://drummercdn.buffolollo.it/undefined`.
- The upload destination folder keeps the original extension case
  (`Song.MP3` goes to `uploads/MP3`), and missing folders are not created.
- Filenames are sanitized by replacing each `[<>:"/\\|?*\s]` character with `_`,
  without collapsing consecutive replacements.
- Oversized files and unexpected/second file fields produce Multer-style 400
  errors; invalid extensions, missing folders, and malformed multipart bodies
  produce a generic 500 with `"error":{}`.
- If any error occurs after a file was created, that file is removed.
- A wrong `Authorization` token does not produce a response: the request hangs
  until the client disconnects. A missing header returns
  `{"message":"Not Authorized!"}` with HTTP 200.
- The delete-all endpoint is a `GET` and only removes files with valid audio
  extensions, leaving directories and other files in place.
- `GET /upload/files` returns `{"message":"error"}` (no `songs` key) when there
  are no audio files, and includes dotfiles because the listing does not filter
  names beyond the extension check.
- Static serving disables directory indexes and refuses dotfiles/dot-directories
  with 404, while still answering CORS preflights (including on static paths)
  with 204 and `Content-Length: 0`.

## Recommended improvements (NOT applied, to keep behavior identical)

- Return `401 Unauthorized` for a wrong token instead of hanging forever.
- Use `DELETE` (or `POST`) for the destructive delete-all operation instead of
  `GET`.
- Create upload destination folders with `mkdir -p`/`os.MkdirAll` so a
  case-variant extension folder does not fail the upload.
- Reject uploads that contain no file instead of returning a success response
  with an `undefined` URL.
