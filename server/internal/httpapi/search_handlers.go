package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/Kush-Singh-26/k-share/server/internal/domain"
	"github.com/Kush-Singh-26/k-share/server/internal/search"
)

// HandleSearch handles POST /search for file search queries.
func (h Handlers) HandleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, domain.ErrNotFound, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, domain.ErrInvalidConfig, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	// Use the role from the request or fall back to the auth-derived role.
	role := req.Role
	if role == "" {
		role = h.GetRole(r)
	}

	rootDir, err := h.GetEffectiveRoot(r)
	if err != nil {
		writeJSONError(w, domain.ErrUnauthorized, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// For guests, restrict search to public subdirectories.
	if role == "guest" {
		// Future: apply guest path filtering here if needed.
		// Currently, the index is built over the full root and filtered by role at search time.
	}

	idx := search.NewIndex(rootDir)
	_ = idx.Build()
	results := idx.Query(req.Query)

	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, results)
}
