package handlers

import (
	"net/http"

	"github.com/erhan/falcon/api/internal/models"
	"github.com/jmoiron/sqlx"
)

type RunsHandler struct {
	DB *sqlx.DB
}

// ListByProgram paginates pipeline runs for a program.
//
//	@Summary		List runs
//	@Tags			runs
//	@Produce		json
//	@Param			id			path		int	true	"program id"
//	@Param			page		query		int	false	"page"
//	@Param			page_size	query		int	false	"page size"
//	@Success		200			{object}	pageResp
//	@Failure		500			{object}	errorResp
//	@Security		BearerAuth
//	@Router			/api/programs/{id}/runs [get]
func (h *RunsHandler) ListByProgram(w http.ResponseWriter, r *http.Request) {
	pid, err := paramInt(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	pg, offset := parsePagination(r, 25, 200)

	var total int
	if err := h.DB.GetContext(r.Context(), &total,
		`SELECT COUNT(*) FROM runs WHERE program_id=$1`, pid); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	var rows []models.Run
	if err := h.DB.SelectContext(r.Context(), &rows, `
SELECT r.id, r.scope_id, COALESCE(s.value, '') AS scope_value,
       r.program_id, r.trigger, r.status, r.started_at, r.finished_at,
       r.error, r.created_at
FROM runs r
LEFT JOIN scope s ON s.id = r.scope_id
WHERE r.program_id=$1
ORDER BY r.created_at DESC
LIMIT $2 OFFSET $3`,
		pid, pg.PageSize, offset); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, pageResponse(rows, total, pg))
}

// Delete removes a single run row and its pipeline steps. Hosts, services
// and endpoints discovered during the run are *not* deleted — they live in
// their own tables with no FK back to runs, so they remain in the asset
// graph for future scans to update.
//
//	@Summary		Delete run
//	@Tags			runs
//	@Param			id	path	int	true	"run id"
//	@Success		204
//	@Failure		404	{object}	errorResp
//	@Security		BearerAuth
//	@Router			/api/runs/{id} [delete]
func (h *RunsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := h.DB.ExecContext(r.Context(), `DELETE FROM runs WHERE id=$1`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DeleteAllForProgram wipes every run (and its steps) for a program. Asset
// data is preserved.
//
//	@Summary		Delete all runs for a program
//	@Tags			runs
//	@Param			id	path		int	true	"program id"
//	@Success		200	{object}	map[string]interface{}
//	@Failure		500	{object}	errorResp
//	@Security		BearerAuth
//	@Router			/api/programs/{id}/runs [delete]
func (h *RunsHandler) DeleteAllForProgram(w http.ResponseWriter, r *http.Request) {
	pid, err := paramInt(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := h.DB.ExecContext(r.Context(), `DELETE FROM runs WHERE program_id=$1`, pid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	n, _ := res.RowsAffected()
	writeJSON(w, http.StatusOK, map[string]any{"deleted": n})
}

// Get returns a run with its pipeline steps.
//
//	@Summary		Get run with steps
//	@Tags			runs
//	@Produce		json
//	@Param			id	path		int	true	"run id"
//	@Success		200	{object}	map[string]interface{}
//	@Failure		404	{object}	errorResp
//	@Security		BearerAuth
//	@Router			/api/runs/{id} [get]
func (h *RunsHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var run models.Run
	if err := h.DB.GetContext(r.Context(), &run, `
SELECT r.id, r.scope_id, COALESCE(s.value, '') AS scope_value,
       r.program_id, r.trigger, r.status, r.started_at, r.finished_at,
       r.error, r.created_at
FROM runs r
LEFT JOIN scope s ON s.id = r.scope_id
WHERE r.id=$1`, id); err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	var steps []models.RunStep
	if err := h.DB.SelectContext(r.Context(), &steps,
		`SELECT * FROM run_steps WHERE run_id=$1 ORDER BY id ASC`, id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": run, "steps": steps})
}
