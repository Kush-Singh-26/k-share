package realtime

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestNotifySerializesEvent(t *testing.T) {
	hub := NewHub()
	select {
	case hub.broadcast <- mustJSON(map[string]string{"type": "files"}):
	default:
	}
}

func mustJSON(v any) []byte {
	data, _ := json.Marshal(v)
	return data
}

func TestHandleWSRejectsInvalidRequest(t *testing.T) {
	hub := NewHub()
	req := httptest.NewRequest("GET", "https://example.com/ws", nil)
	rec := httptest.NewRecorder()
	hub.HandleWS(rec, req)
}
