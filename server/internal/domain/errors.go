package domain

// AppError is a structured error sent to clients so they can show consistent messages.
type AppError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e AppError) Error() string { return e.Message }

const (
	ErrUnauthorized      = "unauthorized"
	ErrForbidden         = "forbidden"
	ErrTrustRequired     = "trust_required"
	ErrTrustMismatch     = "trust_mismatch"
	ErrServerUnreachable = "server_unreachable"
	ErrDiscoveryFailed   = "discovery_failed"
	ErrInvalidConfig     = "invalid_config"
	ErrTransferFailed    = "transfer_failed"
	ErrInvalidPath       = "invalid_path"
	ErrRateLimitExceeded = "rate_limit_exceeded"
	ErrNotFound          = "not_found"
)
