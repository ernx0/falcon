package handlers

import (
	"net/http"

	"github.com/erhan/falcon/api/internal/models"
	"github.com/erhan/falcon/api/internal/scope"
	"github.com/jmoiron/sqlx"
)

type OOSHandler struct {
	DB    *sqlx.DB
	Cache *scope.Cache
}

type oosReq struct {
	Pattern string `json:"pattern"`
	Kind    string `json:"kind"` // exact|wildcard|regex
}

// List returns out-of-scope rules for a program.
//
//	@Summary		List out-of-scope rules
//	@Tags			oos
//	@Produce		json
//	@Param			id			path		int	true	"program id"
//	@Param			page		query		int	false	"page"
//	@Param			page_size	query		int	false	"page size"
//	@Success		200			{object}	pageResp
//	@Failure		500			{object}	errorResp
//	@Security		BearerAuth
//	@Router			/api/programs/{id}/oos [get]
func (h *OOSHandler) List(w http.ResponseWriter, r *http.Request) {
	pid, err := paramInt(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	pg, offset := parsePagination(r, 50, 200)

	var total int
	if err := h.DB.GetContext(r.Context(), &total,
		`SELECT COUNT(*) FROM out_of_scopes WHERE program_id=$1`, pid); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	var rows []models.OutOfScope
	if err := h.DB.SelectContext(r.Context(), &rows,
		`SELECT * FROM out_of_scopes WHERE program_id=$1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		pid, pg.PageSize, offset); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, pageResponse(rows, total, pg))
}

// Create adds an out-of-scope rule. Kind is auto-detected when omitted.
//
//	@Summary		Add out-of-scope rule
//	@Tags			oos
//	@Accept			json
//	@Produce		json
//	@Param			id		path		int			true	"program id"
//	@Param			body	body		oosReq	true	"rule body"
//	@Success		201		{object}	models.OutOfScope
//	@Failure		400		{object}	errorResp
//	@Security		BearerAuth
//	@Router			/api/programs/{id}/oos [post]
func (h *OOSHandler) Create(w http.ResponseWriter, r *http.Request) {
	pid, err := paramInt(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var req oosReq
	if err := decode(r, &req); err != nil || req.Pattern == "" {
		writeErr(w, http.StatusBadRequest, "pattern required")
		return
	}
	if req.Kind == "" {
		if len(req.Pattern) > 1 && req.Pattern[0] == '*' {
			req.Kind = "wildcard"
		} else {
			req.Kind = "exact"
		}
	}
	switch req.Kind {
	case "exact", "wildcard", "regex":
	default:
		writeErr(w, http.StatusBadRequest, "kind must be exact|wildcard|regex")
		return
	}
	var s models.OutOfScope
	err = h.DB.GetContext(r.Context(), &s, `
INSERT INTO out_of_scopes(program_id,pattern,kind) VALUES($1,$2,$3)
RETURNING *`, pid, req.Pattern, req.Kind)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	h.Cache.Invalidate(pid)
	writeJSON(w, http.StatusCreated, s)
}

// Delete removes an out-of-scope rule.
//
//	@Summary		Delete out-of-scope rule
//	@Tags			oos
//	@Param			id	path	int	true	"rule id"
//	@Success		204
//	@Failure		404	{object}	errorResp
//	@Security		BearerAuth
//	@Router			/api/oos/{id} [delete]
func (h *OOSHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var pid int64
	if err := h.DB.GetContext(r.Context(), &pid,
		`DELETE FROM out_of_scopes WHERE id=$1 RETURNING program_id`, id); err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	h.Cache.Invalidate(pid)
	w.WriteHeader(http.StatusNoContent)
}

// Helper to load matcher for a program (used by asset upsert).
func LoadMatcher(db *sqlx.DB, cache *scope.Cache, programID int64) (*scope.Matcher, error) {
	if m, ok := cache.Get(programID); ok {
		return m, nil
	}
	var rows []models.OutOfScope
	if err := db.Select(&rows,
		`SELECT * FROM out_of_scopes WHERE program_id=$1`, programID); err != nil {
		return nil, err
	}
	pats := make([]scope.Pattern, 0, len(rows))
	for _, r := range rows {
		pats = append(pats, scope.Pattern{Kind: r.Kind, Pattern: r.Pattern})
	}
	m := scope.NewMatcher(pats)
	cache.Set(programID, m)
	return m, nil
}
