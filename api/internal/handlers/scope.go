package handlers

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/erhan/falcon/api/internal/jobs"
	"github.com/erhan/falcon/api/internal/models"
	"github.com/jmoiron/sqlx"
	"github.com/robfig/cron/v3"
)

type ScopeHandler struct {
	DB     *sqlx.DB
	Jobs   *jobs.Client
	Parser cron.Parser
}

func NewScopeHandler(db *sqlx.DB, j *jobs.Client) *ScopeHandler {
	return &ScopeHandler{
		DB:     db,
		Jobs:   j,
		Parser: cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow),
	}
}

type scopeReq struct {
	Value        string  `json:"value"`
	Kind         string  `json:"kind"` // domain|wildcard|ip|cidr
	ScheduleCron *string `json:"schedule_cron"`
	Enabled      *bool   `json:"enabled"`
}

func detectKind(v string) string {
	// Any wildcard (classic "*." or label-inner "prod-*.") is a wildcard scope.
	if strings.Contains(v, "*") {
		return "wildcard"
	}
	if strings.Contains(v, "/") {
		return "cidr"
	}
	return "domain"
}

// isScannableKind returns true for scope kinds the worker pipeline can
// actually scan. Other kinds (ios/android/github/etc.) ride along in the
// scope table for record-keeping but the scheduler & run-all skip them.
func isScannableKind(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "domain", "wildcard", "ip", "cidr", "host", "":
		return true
	}
	return false
}

// List paginates targets for a program. By default only returns scannable
// rows (host/wildcard/CIDR/IP); pass scope=all to include descriptive
// non-host items (mobile apps, repos, labels) — used by the Scope tab.
//
//	@Summary		List scope items
//	@Tags			scope
//	@Produce		json
//	@Param			id			path		int		true	"program id"
//	@Param			scope		query		string	false	"scannable | all"
//	@Param			page		query		int		false	"page"
//	@Param			page_size	query		int		false	"page size"
//	@Success		200			{object}	pageResp
//	@Failure		500			{object}	errorResp
//	@Security		BearerAuth
//	@Router			/api/programs/{id}/scope [get]
func (h *ScopeHandler) List(w http.ResponseWriter, r *http.Request) {
	pid, err := paramInt(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	pg, offset := parsePagination(r, 25, 200)

	scope := r.URL.Query().Get("scope")
	scannableFilter := scope != "all" // default: scannable only
	whereExtra := ""
	if scannableFilter {
		whereExtra = " AND kind IN ('domain','wildcard','ip','cidr','host','')"
	}

	var total int
	if err := h.DB.GetContext(r.Context(), &total,
		`SELECT COUNT(*) FROM scope WHERE program_id=$1`+whereExtra, pid); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	var rows []models.ScopeItem
	if err := h.DB.SelectContext(r.Context(), &rows,
		`SELECT * FROM scope WHERE program_id=$1`+whereExtra+
			` ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		pid, pg.PageSize, offset); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]models.ScopeItemJSON, 0, len(rows))
	for _, it := range rows {
		out = append(out, it.ToJSON())
	}
	writeJSON(w, http.StatusOK, pageResponse(out, total, pg))
}

// Create inserts a target. Kind is auto-detected from the value when omitted.
//
//	@Summary		Create scope item
//	@Tags			scope
//	@Accept			json
//	@Produce		json
//	@Param			id		path		int			true	"program id"
//	@Param			body	body		scopeReq	true	"scope item body"
//	@Success		201		{object}	models.ScopeItemJSON
//	@Failure		400		{object}	errorResp
//	@Security		BearerAuth
//	@Router			/api/programs/{id}/scope [post]
func (h *ScopeHandler) Create(w http.ResponseWriter, r *http.Request) {
	pid, err := paramInt(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var req scopeReq
	if err := decode(r, &req); err != nil || req.Value == "" {
		writeErr(w, http.StatusBadRequest, "value required")
		return
	}
	if req.Kind == "" {
		req.Kind = detectKind(req.Value)
	}
	if req.ScheduleCron != nil && *req.ScheduleCron != "" {
		if _, err := h.Parser.Parse(*req.ScheduleCron); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid cron: "+err.Error())
			return
		}
	}
	cronVal := sql.NullString{}
	if req.ScheduleCron != nil && *req.ScheduleCron != "" {
		cronVal = sql.NullString{String: *req.ScheduleCron, Valid: true}
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	var item models.ScopeItem
	err = h.DB.GetContext(r.Context(), &item, `
INSERT INTO scope(program_id,value,kind,schedule_cron,enabled,next_run_at)
VALUES($1,$2,$3,$4::text,$5, CASE WHEN $4::text IS NOT NULL THEN NOW() ELSE NULL END)
RETURNING *`, pid, req.Value, req.Kind, cronVal, enabled)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, item.ToJSON())
}

// Update mutates a target's schedule / enabled flag.
//
//	@Summary		Update scope item
//	@Tags			scope
//	@Accept			json
//	@Produce		json
//	@Param			id		path		int			true	"scope item id"
//	@Param			body	body		scopeReq	true	"fields"
//	@Success		200		{object}	models.ScopeItemJSON
//	@Failure		400		{object}	errorResp
//	@Failure		404		{object}	errorResp
//	@Security		BearerAuth
//	@Router			/api/scope/{id} [patch]
func (h *ScopeHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var req scopeReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.ScheduleCron != nil && *req.ScheduleCron != "" {
		if _, err := h.Parser.Parse(*req.ScheduleCron); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid cron: "+err.Error())
			return
		}
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	cronVal := sql.NullString{}
	if req.ScheduleCron != nil {
		if *req.ScheduleCron != "" {
			cronVal = sql.NullString{String: *req.ScheduleCron, Valid: true}
		}
	}
	var item models.ScopeItem
	err = h.DB.GetContext(r.Context(), &item, `
UPDATE scope SET
  schedule_cron=$2,
  enabled=$3
WHERE id=$1 RETURNING *`, id, cronVal, enabled)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, item.ToJSON())
}

// Delete removes a target.
//
//	@Summary		Delete scope item
//	@Tags			scope
//	@Param			id	path	int	true	"scope item id"
//	@Success		204
//	@Failure		500	{object}	errorResp
//	@Security		BearerAuth
//	@Router			/api/scope/{id} [delete]
func (h *ScopeHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := h.DB.ExecContext(r.Context(), `DELETE FROM scope WHERE id=$1`, id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Run manually triggers a recon pipeline for a single scope item.
//
//	@Summary		Trigger pipeline run
//	@Tags			scope
//	@Produce		json
//	@Param			id	path		int	true	"scope item id"
//	@Success		202	{object}	runStartResp
//	@Failure		404	{object}	errorResp
//	@Failure		500	{object}	errorResp
//	@Security		BearerAuth
//	@Router			/api/scope/{id}/run [post]
func (h *ScopeHandler) Run(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var item models.ScopeItem
	if err := h.DB.GetContext(r.Context(), &item,
		`SELECT * FROM scope WHERE id=$1`, id); err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if !isScannableKind(item.Kind) {
		writeErr(w, http.StatusBadRequest,
			"this scope item's kind ("+item.Kind+") isn't scannable; only host/wildcard/CIDR/IP can be run")
		return
	}
	var runID int64
	if err := h.DB.GetContext(r.Context(), &runID, `
INSERT INTO runs(scope_id,program_id,trigger,status) VALUES($1,$2,'manual','queued')
RETURNING id`, item.ID, item.ProgramID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.Jobs.EnqueuePipeline(r.Context(), runID, item.ProgramID, item.ID, item.Value, item.Kind); err != nil {
		writeErr(w, http.StatusInternalServerError, "enqueue: "+err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"run_id": runID, "status": "queued"})
}

// RunAll enqueues a manual recon run for every enabled target in a program.
// Returns the count of runs enqueued. Disabled targets are skipped. We
// stream-fetch + insert in a single transaction so a partial failure leaves
// no half-queued state.
//
//	@Summary		Trigger pipeline for every scope item in a program
//	@Tags			scope
//	@Produce		json
//	@Param			id	path		int	true	"program id"
//	@Success		202	{object}	map[string]interface{}
//	@Failure		500	{object}	errorResp
//	@Security		BearerAuth
//	@Router			/api/programs/{id}/scope/run-all [post]
func (h *ScopeHandler) RunAll(w http.ResponseWriter, r *http.Request) {
	pid, err := paramInt(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var items []models.ScopeItem
	if err := h.DB.SelectContext(r.Context(), &items,
		`SELECT * FROM scope WHERE program_id=$1 AND enabled ORDER BY id`, pid); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(items) == 0 {
		writeJSON(w, http.StatusAccepted, map[string]any{"queued": 0, "run_ids": []int64{}})
		return
	}

	type queued struct {
		RunID    int64  `json:"run_id"`
		ScopeID int64  `json:"scope_id"`
		Value    string `json:"value"`
	}
	out := make([]queued, 0, len(items))
	failed := 0
	for _, item := range items {
		var runID int64
		if err := h.DB.GetContext(r.Context(), &runID, `
INSERT INTO runs(scope_id,program_id,trigger,status) VALUES($1,$2,'manual','queued')
RETURNING id`, item.ID, item.ProgramID); err != nil {
			failed++
			continue
		}
		if err := h.Jobs.EnqueuePipeline(r.Context(), runID, item.ProgramID, item.ID, item.Value, item.Kind); err != nil {
			failed++
			continue
		}
		out = append(out, queued{RunID: runID, ScopeID: item.ID, Value: item.Value})
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"queued":     len(out),
		"failed":     failed,
		"total":      len(items),
		"run_ids":    out,
	})
}
