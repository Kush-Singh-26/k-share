package domain

// SearchResult represents a single file or directory matching a search query.
type SearchResult struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	IsDirectory bool   `json:"isDirectory"`
	Size        int64  `json:"size"`
	ModTime     string `json:"modTime"`
}

// SearchRequest is the body of a POST /search request.
type SearchRequest struct {
	Query string `json:"query"`
	Role  string `json:"role"`
}
