package handlers

import (
	"net/http"

	"github.com/erhan/falcon/api/internal/models"
	"github.com/jmoiron/sqlx"
)

type ProgramsHandler struct {
	DB *sqlx.DB
}

type programReq struct {
	Name        string  `json:"name"`
	Slug        string  `json:"slug"`
	Description string  `json:"description"`
	Rules       *string `json:"rules"`
	IconURL     *string `json:"icon_url"`
	Platform    string  `json:"platform"`
}

// List returns a page of programs with target / open-finding counts.
//
//	@Summary		List programs
//	@Tags			programs
//	@Produce		json
//	@Param			page		query		int	false	"page (1-based)"
//	@Param			page_size	query		int	false	"page size"
//	@Success		200			{object}	pageResp
//	@Failure		500			{object}	errorResp
//	@Security		BearerAuth
//	@Router			/api/programs [get]
func (h *ProgramsHandler) List(w http.ResponseWriter, r *http.Request) {
	pg, offset := parsePagination(r, 24, 100)

	var total int
	if err := h.DB.GetContext(r.Context(), &total, `SELECT COUNT(*) FROM programs`); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	var rows []struct {
		models.Program
		TargetCount  int `db:"target_count" json:"target_count"`
		ReportsOpen  int `db:"reports_open" json:"reports_open"`
	}
	q := `
SELECT p.*,
  (SELECT COUNT(*) FROM scope t WHERE t.program_id=p.id) AS target_count,
  (SELECT COUNT(*) FROM reports f WHERE f.program_id=p.id AND f.status NOT IN ('resolved','duplicate','n_a')) AS reports_open
FROM programs p ORDER BY p.created_at DESC LIMIT $1 OFFSET $2`
	if err := h.DB.SelectContext(r.Context(), &rows, q, pg.PageSize, offset); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, pageResponse(rows, total, pg))
}

// Create inserts a new program.
//
//	@Summary		Create program
//	@Tags			programs
//	@Accept			json
//	@Produce		json
//	@Param			body	body		programReq	true	"program body"
//	@Success		201		{object}	models.Program
//	@Failure		400		{object}	errorResp
//	@Security		BearerAuth
//	@Router			/api/programs [post]
func (h *ProgramsHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req programReq
	if err := decode(r, &req); err != nil || req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name required")
		return
	}
	if req.Slug == "" {
		req.Slug = slugify(req.Name)
	} else {
		req.Slug = slugify(req.Slug)
	}
	icon := ""
	if req.IconURL != nil {
		icon = *req.IconURL
	}
	var p models.Program
	err := h.DB.GetContext(r.Context(), &p, `
INSERT INTO programs(name,slug,description,platform,icon_url) VALUES($1,$2,$3,$4,$5)
RETURNING *`, req.Name, req.Slug, req.Description, req.Platform, icon)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

// Get fetches a program by id.
//
//	@Summary		Get program
//	@Tags			programs
//	@Produce		json
//	@Param			id	path		int	true	"program id"
//	@Success		200	{object}	models.Program
//	@Failure		404	{object}	errorResp
//	@Security		BearerAuth
//	@Router			/api/programs/{id} [get]
func (h *ProgramsHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var p models.Program
	if err := h.DB.GetContext(r.Context(), &p, `SELECT * FROM programs WHERE id=$1`, id); err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// Update mutates a program.
//
//	@Summary		Update program
//	@Tags			programs
//	@Accept			json
//	@Produce		json
//	@Param			id		path		int			true	"program id"
//	@Param			body	body		programReq	true	"fields to update"
//	@Success		200		{object}	models.Program
//	@Failure		404		{object}	errorResp
//	@Security		BearerAuth
//	@Router			/api/programs/{id} [patch]
func (h *ProgramsHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var req programReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var p models.Program
	err = h.DB.GetContext(r.Context(), &p, `
UPDATE programs SET
  name=COALESCE(NULLIF($2,''),name),
  description=$3,
  rules=COALESCE($4, rules),
  icon_url=COALESCE($5, icon_url),
  platform=$6,
  updated_at=NOW()
WHERE id=$1 RETURNING *`, id, req.Name, req.Description, req.Rules, req.IconURL, req.Platform)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// Delete removes a program and its dependent rows.
//
//	@Summary		Delete program
//	@Tags			programs
//	@Param			id	path	int	true	"program id"
//	@Success		204
//	@Failure		500	{object}	errorResp
//	@Security		BearerAuth
//	@Router			/api/programs/{id} [delete]
func (h *ProgramsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := h.DB.ExecContext(r.Context(), `DELETE FROM programs WHERE id=$1`, id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
