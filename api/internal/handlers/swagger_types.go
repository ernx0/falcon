package handlers

// These types exist purely so swag can resolve schemas in handler annotations.
// They are not used by runtime code paths.

// errorResp is the body returned by writeErr.
type errorResp struct {
	Error string `json:"error"`
}

// pageResp is the generic paginated list envelope returned by pageResponse.
type pageResp struct {
	Items    []any `json:"items"`
	Total    int   `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}

// runStartResp is returned by POST /api/targets/{id}/run.
type runStartResp struct {
	RunID  int64  `json:"run_id"`
	Status string `json:"status"`
}
