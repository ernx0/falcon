package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"github.com/erhan/falcon/api/internal/models"
	"github.com/jmoiron/sqlx"
)

type ReportsHandler struct {
	DB *sqlx.DB
}

// reportReq is the JSON body accepted by Create/Update. Every
// updatable string is a pointer so PATCH can distinguish "field
// omitted" (nil → leave alone) from "field set to empty" (&"" → write
// empty). Without this you can't, e.g., update only `target` without
// also wiping the description.
//
// Severity is intentionally not present here: it's derived from `vrt`
// on the server (see deriveSeverity) and any client-supplied value
// would be ignored anyway. Dropping the field from the contract keeps
// the OpenAPI schema honest.
type reportReq struct {
	HostID         *int64   `json:"host_id"`
	ServiceID      *int64   `json:"service_id"`
	EndpointID     *int64   `json:"endpoint_id"`
	Title          *string  `json:"title"`
	Target         *string  `json:"target"`
	VRT            *string  `json:"vrt"`
	URL            *string  `json:"url"`
	Status         *string  `json:"status"`
	DescriptionMD  *string  `json:"description_md"`
	PoC            *string  `json:"poc"`
	CVSS           *string  `json:"cvss"`
	ExternalURL    *string  `json:"external_url"`
	RewardAmount   *float64 `json:"reward_amount"`
	RewardCurrency *string  `json:"reward_currency"`
}

// strOr returns *p when non-nil, else fallback. Used to read optional
// string fields with a default (Create defaults).
func strOr(p *string, fallback string) string {
	if p != nil {
		return *p
	}
	return fallback
}

var validSev = map[string]bool{"none": true, "info": true, "low": true, "medium": true, "high": true, "critical": true}
var validStatus = map[string]bool{"draft": true, "submitted": true, "triaged": true, "resolved": true, "duplicate": true, "n_a": true}

// vrtPriorityFor walks the VRT taxonomy and looks up the priority for a
// dotted id path like
// "ai_application_security.adversarial_example_injection.ai_misclassification_attacks".
// Returns 0 when the id is unknown or the node has no explicit priority.
//
// The index is lazily built from the currently active VRT bytes (embedded by
// default; overridden by VRTHandler.Refresh). It can be rebuilt by calling
// resetVRTIndex with a fresh payload.
var (
	vrtIndexMu    sync.RWMutex
	vrtIndexBuilt bool
	vrtIndex      map[string]int
)

func buildVRTIndexFrom(raw []byte) map[string]int {
	idx := map[string]int{}
	var doc struct {
		Content []json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return idx
	}
	var walk func(items []json.RawMessage, prefix string)
	walk = func(items []json.RawMessage, prefix string) {
		for _, r := range items {
			var node struct {
				ID       string            `json:"id"`
				Priority *int              `json:"priority"`
				Children []json.RawMessage `json:"children"`
			}
			if err := json.Unmarshal(r, &node); err != nil {
				continue
			}
			if node.ID == "" {
				continue
			}
			path := node.ID
			if prefix != "" {
				path = prefix + "." + node.ID
			}
			if node.Priority != nil {
				idx[path] = *node.Priority
			}
			if len(node.Children) > 0 {
				walk(node.Children, path)
			}
		}
	}
	walk(doc.Content, "")
	return idx
}

func resetVRTIndex(raw []byte) {
	idx := buildVRTIndexFrom(raw)
	vrtIndexMu.Lock()
	vrtIndex = idx
	vrtIndexBuilt = true
	vrtIndexMu.Unlock()
}

func vrtPriorityFor(id string) int {
	id = strings.TrimSpace(id)
	if id == "" {
		return 0
	}
	vrtIndexMu.RLock()
	built := vrtIndexBuilt
	vrtIndexMu.RUnlock()
	if !built {
		resetVRTIndex(vrtJSON)
	}
	vrtIndexMu.RLock()
	defer vrtIndexMu.RUnlock()
	return vrtIndex[id]
}

// severityFromVRT maps a Bugcrowd VRT priority (1=highest) to our severity buckets.
func severityFromVRT(vrtID string) string {
	switch vrtPriorityFor(vrtID) {
	case 1:
		return "critical"
	case 2:
		return "high"
	case 3:
		return "medium"
	case 4:
		return "low"
	case 5:
		return "info"
	}
	return ""
}

// deriveSeverity is the single source of truth for a report's severity:
// always derived from the VRT id, never accepted from clients. Returns
// "none" when the VRT id is empty or the node has no priority — that's
// distinct from "info" (P5), which is the lowest *explicit* bucket.
func deriveSeverity(vrtID string) string {
	if s := severityFromVRT(vrtID); s != "" {
		return s
	}
	return "none"
}

// RecomputeAllReportSeverities walks every report and rewrites severity
// from its current VRT id. Idempotent — only updates rows that drifted.
// Called once on API startup so legacy data (where severity could be
// set independently) realigns with the taxonomy.
func RecomputeAllReportSeverities(ctx context.Context, db *sqlx.DB) (int, error) {
	type row struct {
		ID       int64  `db:"id"`
		VRT      string `db:"vrt"`
		Severity string `db:"severity"`
	}
	var rows []row
	if err := db.SelectContext(ctx, &rows, `SELECT id, vrt, severity FROM reports`); err != nil {
		return 0, err
	}
	updated := 0
	for _, r := range rows {
		want := deriveSeverity(r.VRT)
		if want == r.Severity {
			continue
		}
		if _, err := db.ExecContext(ctx,
			`UPDATE reports SET severity=$1 WHERE id=$2`, want, r.ID); err != nil {
			return updated, err
		}
		updated++
	}
	return updated, nil
}

// List paginates findings for a program.
//
//	@Summary		List reports
//	@Tags			reports
//	@Produce		json
//	@Param			id			path		int	true	"program id"
//	@Param			page		query		int	false	"page"
//	@Param			page_size	query		int	false	"page size"
//	@Success		200			{object}	pageResp
//	@Failure		500			{object}	errorResp
//	@Security		BearerAuth
//	@Router			/api/programs/{id}/reports [get]
func (h *ReportsHandler) List(w http.ResponseWriter, r *http.Request) {
	pid, err := paramInt(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	pg, offset := parsePagination(r, 25, 200)

	var total int
	if err := h.DB.GetContext(r.Context(), &total,
		`SELECT COUNT(*) FROM reports WHERE program_id=$1`, pid); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	var rows []models.Report
	if err := h.DB.SelectContext(r.Context(), &rows,
		`SELECT * FROM reports WHERE program_id=$1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		pid, pg.PageSize, offset); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, pageResponse(rows, total, pg))
}

// Create adds a finding to a program.
//
//	@Summary		Create report
//	@Tags			reports
//	@Accept			json
//	@Produce		json
//	@Param			id		path		int			true	"program id"
//	@Param			body	body		reportReq	true	"report body"
//	@Success		201		{object}	models.Report
//	@Failure		400		{object}	errorResp
//	@Security		BearerAuth
//	@Router			/api/programs/{id}/reports [post]
func (h *ReportsHandler) Create(w http.ResponseWriter, r *http.Request) {
	pid, err := paramInt(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var req reportReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	title := strOr(req.Title, "")
	if title == "" {
		writeErr(w, http.StatusBadRequest, "title required")
		return
	}
	vrt := strOr(req.VRT, "")
	severity := deriveSeverity(vrt)
	status := strOr(req.Status, "draft")
	if !validStatus[status] {
		writeErr(w, http.StatusBadRequest, "invalid status")
		return
	}
	host := nullInt64(req.HostID)
	service := nullInt64(req.ServiceID)
	endpoint := nullInt64(req.EndpointID)
	reward := sql.NullFloat64{}
	if req.RewardAmount != nil {
		reward = sql.NullFloat64{Float64: *req.RewardAmount, Valid: true}
	}
	var rpt models.Report
	err = h.DB.GetContext(r.Context(), &rpt, `
INSERT INTO reports(program_id,host_id,service_id,endpoint_id,title,target,vrt,url,severity,status,description_md,poc,cvss,external_url,reward_amount,reward_currency)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16) RETURNING *`,
		pid, host, service, endpoint,
		title, strOr(req.Target, ""), vrt, strOr(req.URL, ""),
		severity, status,
		strOr(req.DescriptionMD, ""), strOr(req.PoC, ""),
		strOr(req.CVSS, ""), strOr(req.ExternalURL, ""),
		reward, strOr(req.RewardCurrency, ""))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, rpt)
}

// Get returns a single finding by id.
//
//	@Summary		Get report
//	@Tags			reports
//	@Produce		json
//	@Param			id	path		int	true	"report id"
//	@Success		200	{object}	models.Report
//	@Failure		404	{object}	errorResp
//	@Security		BearerAuth
//	@Router			/api/reports/{id} [get]
func (h *ReportsHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var rpt models.Report
	if err := h.DB.GetContext(r.Context(), &rpt, `SELECT * FROM reports WHERE id=$1`, id); err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, rpt)
}

// Update mutates fields on a finding.
//
//	@Summary		Update report
//	@Tags			reports
//	@Accept			json
//	@Produce		json
//	@Param			id		path		int			true	"report id"
//	@Param			body	body		reportReq	true	"fields"
//	@Success		200		{object}	models.Report
//	@Failure		400		{object}	errorResp
//	@Failure		404		{object}	errorResp
//	@Security		BearerAuth
//	@Router			/api/reports/{id} [patch]
func (h *ReportsHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var req reportReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Status != nil && !validStatus[*req.Status] {
		writeErr(w, http.StatusBadRequest, "invalid status")
		return
	}
	// Severity rides on VRT — only set it when the client is touching VRT
	// in this PATCH. Otherwise leave it (and `vrt`) alone in the row.
	var sevPtr *string
	if req.VRT != nil {
		s := deriveSeverity(*req.VRT)
		sevPtr = &s
	}
	var reward sql.NullFloat64
	if req.RewardAmount != nil {
		reward = sql.NullFloat64{Float64: *req.RewardAmount, Valid: true}
	}
	// COALESCE($N, column) keeps the existing column value when the
	// caller passes nil for that field — the partial-update contract.
	var rpt models.Report
	err = h.DB.GetContext(r.Context(), &rpt, `
UPDATE reports SET
  title           = COALESCE($2,  title),
  target          = COALESCE($3,  target),
  vrt             = COALESCE($4,  vrt),
  url             = COALESCE($5,  url),
  severity        = COALESCE($6,  severity),
  status          = COALESCE($7,  status),
  description_md  = COALESCE($8,  description_md),
  poc             = COALESCE($9,  poc),
  cvss            = COALESCE($10, cvss),
  external_url    = COALESCE($11, external_url),
  reward_amount   = COALESCE($12, reward_amount),
  reward_currency = COALESCE($13, reward_currency),
  updated_at      = NOW()
WHERE id=$1 RETURNING *`,
		id,
		req.Title, req.Target, req.VRT, req.URL,
		sevPtr, req.Status,
		req.DescriptionMD, req.PoC, req.CVSS, req.ExternalURL,
		reward, req.RewardCurrency)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, rpt)
}

// Delete removes a finding.
//
//	@Summary		Delete report
//	@Tags			reports
//	@Param			id	path	int	true	"report id"
//	@Success		204
//	@Failure		500	{object}	errorResp
//	@Security		BearerAuth
//	@Router			/api/reports/{id} [delete]
func (h *ReportsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := h.DB.ExecContext(r.Context(), `DELETE FROM reports WHERE id=$1`, id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
