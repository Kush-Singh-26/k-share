package auth

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	serverconfig "github.com/Kush-Singh-26/k-share/server/internal/config"
)

func Role(r *http.Request, cfg *serverconfig.Config) string {
	auth := r.Header.Get("Authorization")
	code := ""
	if auth != "" {
		code = strings.TrimPrefix(auth, "Bearer ")
	} else {
		// Fallback to query parameter for WebSockets
		code = r.URL.Query().Get("token")
	}

	if code == "" {
		return "none"
	}

	if subtle.ConstantTimeCompare([]byte(code), []byte(cfg.AdminCode)) == 1 {
		return "admin"
	}
	if subtle.ConstantTimeCompare([]byte(code), []byte(cfg.GuestCode)) == 1 {
		return "guest"
	}
	return "none"
}

func EffectiveRoot(r *http.Request, cfg *serverconfig.Config) (string, error) {
	switch Role(r, cfg) {
	case "admin":
		return cfg.SharedDir, nil
	case "guest":
		return filepath.Join(cfg.SharedDir, "Public"), nil
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
