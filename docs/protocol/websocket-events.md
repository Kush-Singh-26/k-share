# WebSocket Events

> **Version**: 1.0
> **Last Updated**: 2026-04-25

The server uses a small typed event stream so clients can refresh state without polling.

Event format:

```json
{
  "type": "files"
}
```

## Current event types

- `files`: file list or file tree changed
- `clip`: main clipboard text changed
- `clip_guest`: guest clipboard text changed
- `clip_image`: clipboard image changed
- `history`: clipboard history changed

## Client behavior

- Desktop clients should refresh the matching visible section for the event type.
- Android clients currently react to clipboard events and refresh the relevant sync state.
- Unknown event types should be ignored safely.

## Contract rules

- Event names are stable protocol values, not UI labels.
- New event types should be additive.
- If a breaking rename is ever required, version the event contract first.

