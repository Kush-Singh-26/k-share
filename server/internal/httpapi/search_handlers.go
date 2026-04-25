package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/Kush-Singh-26/k-share/server/internal/domain"
	"github.com/Kush-Singh-26/k-share/server/internal/search"
)

// HandleSearch handles POST /search for file search queries.
func (h *Handlers) HandleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, domain.ErrNotFound, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 4<<10) // 4 KB for search query
	var req domain.SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, domain.ErrInvalidConfig, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	role := h.GetRole(r)
	// Ignore req.Role override for security.

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

	var idx *search.Index
	if role == "admin" {
		if h.GetAdminIndex != nil {
			idx = h.GetAdminIndex()
		} else {
			idx = h.AdminIndex
		}
	} else {
		if h.GetGuestIndex != nil {
			idx = h.GetGuestIndex()
		} else {
			idx = h.GuestIndex
		}
	}

	if idx == nil {
		idx = search.NewIndex(rootDir)
		_ = idx.Build()
		// Note: We don't update h.AdminIndex/GuestIndex here because h is a copy.
		// Rebuilding is handled globally via Hub notify or manual trigger.
		// However, for this request, we use the local one.
	}

	results := idx.Query(req.Query)

	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, results)
}
