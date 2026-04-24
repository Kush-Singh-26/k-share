package auth

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	serverconfig "github.com/Kush-Singh-26/k-share/server/internal/config"
)

func Role(r *http.Request, cfg *serverconfig.Config) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return "none"
	}

	code := strings.TrimPrefix(auth, "Bearer ")
	if code == cfg.AdminCode {
		return "admin"
	}
	if code == cfg.GuestCode {
		return "guest"
	}
	return "none"
}

func EffectiveRoot(r *http.Request, cfg *serverconfig.Config) (string, error) {
	switch Role(r, cfg) {
	case "admin":
		return cfg.SharedDir, nil
	case "guest":
		publicDir := filepath.Join(cfg.SharedDir, "Public")
		if err := os.MkdirAll(publicDir, 0o755); err != nil {
			return "", err
		}
		return publicDir, nil
	default:
		return "", fmt.Errorf("unauthorized")
	}
}

func RequireAuth(next http.HandlerFunc, cfg *serverconfig.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if Role(r, cfg) == "none" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}
