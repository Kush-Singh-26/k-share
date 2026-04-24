# HTTP API

This document describes the current server HTTP contract.

## Authentication

Requests use a bearer code:

```http
Authorization: Bearer <pairing-code>
```

Role resolution:

- `admin`: matches `admin_code`
- `guest`: matches `guest_code`
- unauthorized: any other value or no header

## Endpoints

### `GET /ping`

Purpose:

- lightweight health check
- returns the current role string so clients can confirm auth state

Response:

```json
{
  "status": "ok",
  "name": "K-Share Server",
  "proto": "https",
  "role": "admin"
}
```

Notes:

- CORS is enabled.
- This endpoint is intentionally usable as a discovery and trust probe.

### `GET /files`

Purpose:

- list files visible to the current role

Query parameters:

- `folder` optional relative folder name

Response:

- JSON array of file entries

Rules:

- `admin` can see the full shared root.
- `guest` can only see the public subtree.

### `POST /upload?name=<filename>`

Purpose:

- upload a file stream into the current effective root

Rules:

- requires auth
- path safety is enforced by the server file layer
- on success the server emits a `files` WebSocket event

### `GET /download/{path}`

Purpose:

- download a file or folder export from the current effective root

Rules:

- requires auth
- the request path is relative to the effective root

### `GET /clipboard`

Purpose:

- read clipboard text

Optional query parameters:

- `channel=guest` for the guest clipboard view

Role rules:

- `guest` reads and writes `guest_clipboard.txt`
- `admin` reads and writes `clipboard.txt` unless `channel=guest` is requested

### `POST /clipboard`

Purpose:

- write clipboard text

Optional query parameters:

- `mode=append`
- `channel=guest`

Events:

- `clip` for the main clipboard
- `clip_guest` for the guest clipboard
- `history` when the main clipboard history changes

### `GET /clipboard/image`

Purpose:

- read the shared clipboard image as PNG bytes

### `POST /clipboard/image`

Purpose:

- write the shared clipboard image

Events:

- `clip_image`

### `GET /clipboard/history`

Purpose:

- return the clipboard history list

Rules:

- admin only

### `DELETE /clipboard/history?id=<id>`

Purpose:

- delete one history entry

Rules:

- admin only

### `GET /thumbnail?name=<name>&folder=<folder>`

Purpose:

- return a thumbnail for a file inside the effective root

Rules:

- requires auth
- the server may cache generated thumbnails on disk

### `DELETE /delete?name=<filename>`

Purpose:

- move a file to trash

Rules:

- admin only

### `POST /open`

Purpose:

- open a URL on the server host

Rules:

- only HTTPS URLs are allowed
- non-HTTPS requests are rejected

### `GET /ws`

Purpose:

- WebSocket event stream for clients

Rules:

- requires auth
- sends JSON messages with a `type` field

