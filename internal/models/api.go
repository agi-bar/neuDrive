package models

// APIResponse is a generic API response wrapper.
type APIResponse[T any] struct {
	Data  T      `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
	OK    bool   `json:"ok"`
}

// PaginationParams holds pagination parameters for list endpoints.
type PaginationParams struct {
	Page    int `json:"page"`
	PerPage int `json:"per_page"`
	Offset  int `json:"offset"`
}

// SearchParams holds parameters for search endpoints.
type SearchParams struct {
	Query string `json:"query"`
	Scope string `json:"scope"` // memory, inbox, all
}
