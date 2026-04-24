package auth

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	serverconfig "github.com/Kush-Singh-26/k-share/server/internal/config"
)

func TestRole(t *testing.T) {
	cfg := serverconfig.Config{AdminCode: "admin123", GuestCode: "guest123"}

	req := httptest.NewRequest(http.MethodGet, "https://example.com/ping", nil)
	if got := Role(req, cfg); got != "none" {
		t.Fatalf("Role() without auth = %q, want none", got)
	}

	req.Header.Set("Authorization", "Bearer admin123")
	if got := Role(req, cfg); got != "admin" {
		t.Fatalf("Role() admin = %q, want admin", got)
	}

	req.Header.Set("Authorization", "Bearer guest123")
	if got := Role(req, cfg); got != "guest" {
		t.Fatalf("Role() guest = %q, want guest", got)
	}
}

func TestEffectiveRoot(t *testing.T) {
	cfg := serverconfig.Config{SharedDir: filepath.Join(t.TempDir(), "share"), AdminCode: "admin123", GuestCode: "guest123"}

	adminReq := httptest.NewRequest(http.MethodGet, "https://example.com/files", nil)
	adminReq.Header.Set("Authorization", "Bearer admin123")
	root, err := EffectiveRoot(adminReq, cfg)
	if err != nil {
		t.Fatalf("EffectiveRoot() admin returned error: %v", err)
	}
	if root != cfg.SharedDir {
		t.Fatalf("EffectiveRoot() admin = %q, want %q", root, cfg.SharedDir)
	}

	guestReq := httptest.NewRequest(http.MethodGet, "https://example.com/files", nil)
	guestReq.Header.Set("Authorization", "Bearer guest123")
	root, err = EffectiveRoot(guestReq, cfg)
	if err != nil {
		t.Fatalf("EffectiveRoot() guest returned error: %v", err)
	}
	wantGuestRoot := filepath.Join(cfg.SharedDir, "Public")
	if root != wantGuestRoot {
		t.Fatalf("EffectiveRoot() guest = %q, want %q", root, wantGuestRoot)
	}
}
