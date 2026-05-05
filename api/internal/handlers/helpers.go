package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

func nullInt64(p *int64) sql.NullInt64 {
	if p == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *p, Valid: true}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decode(r *http.Request, v interface{}) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func paramInt(r *http.Request, name string) (int64, error) {
	s := chi.URLParam(r, name)
	if s == "" {
		return 0, errors.New("missing " + name)
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		return 0, errors.New("invalid " + name)
	}
	return n, nil
}

// Pagination is the standard offset-based pagination block. Read from query params.
type Pagination struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
}

// parsePagination reads `page` (1-indexed) and `page_size` from the query string,
// clamps to safe defaults/maxes, and returns the offset to use in SQL.
func parsePagination(r *http.Request, defaultSize, maxSize int) (Pagination, int) {
	page := 1
	if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 {
			page = n
		}
	}
	size := defaultSize
	if v := r.URL.Query().Get("page_size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 {
			if n > maxSize {
				n = maxSize
			}
			size = n
		}
	}
	return Pagination{Page: page, PageSize: size}, (page - 1) * size
}

// pageResponse shapes a paginated list response.
func pageResponse(items any, total int, p Pagination) map[string]any {
	return map[string]any{
		"items":     items,
		"total":     total,
		"page":      p.Page,
		"page_size": p.PageSize,
	}
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		case r == '-' || r == '_' || r == ' ':
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	out := b.String()
	out = strings.TrimRight(out, "-")
	if out == "" {
		out = "program"
	}
	return out
}
