package handlers

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/jmoiron/sqlx"
)

type InternalRunsHandler struct {
	DB           *sqlx.DB
	ArtifactsDir string
}

type startReq struct{}
type stepReq struct {
	Tool         string          `json:"tool"`
	Status       string          `json:"status"` // running|success|failed
	ArtifactPath string          `json:"artifact_path"`
	Stats        json.RawMessage `json:"stats"`
	Error        string          `json:"error"`
}
type finishReq struct {
	Status string `json:"status"` // success|failed|partial
	Error  string `json:"error"`
}

func (h *InternalRunsHandler) Start(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE runs SET status='running', started_at=NOW() WHERE id=$1`, id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *InternalRunsHandler) Step(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var req stepReq
	if err := decode(r, &req); err != nil || req.Tool == "" || req.Status == "" {
		writeErr(w, http.StatusBadRequest, "tool and status required")
		return
	}
	stats := []byte("{}")
	if len(req.Stats) > 0 {
		stats = req.Stats
	}
	var stepID int64
	switch req.Status {
	case "running":
		err = h.DB.GetContext(r.Context(), &stepID, `
INSERT INTO run_steps(run_id,tool,status,started_at) VALUES($1,$2,'running',NOW()) RETURNING id`,
			id, req.Tool)
	default:
		err = h.DB.GetContext(r.Context(), &stepID, `
INSERT INTO run_steps(run_id,tool,status,started_at,finished_at,artifact_path,stats,error)
VALUES($1,$2,$3,NOW(),NOW(),$4,$5,$6) RETURNING id`,
			id, req.Tool, req.Status, req.ArtifactPath, stats, req.Error)
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": stepID})
}

func (h *InternalRunsHandler) Finish(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var req finishReq
	if err := decode(r, &req); err != nil || req.Status == "" {
		writeErr(w, http.StatusBadRequest, "status required")
		return
	}
	if _, err := h.DB.ExecContext(r.Context(), `
UPDATE runs SET status=$2, error=$3, finished_at=NOW() WHERE id=$1`,
		id, req.Status, req.Error); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := h.DB.ExecContext(r.Context(), `
UPDATE scope SET last_run_at=NOW() WHERE id=(SELECT scope_id FROM runs WHERE id=$1)`, id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Artifact serves a file from /data/artifacts.
// Path traversal protected: resolved path must be under ArtifactsDir.
// Accepts either a relative path ("program-1/scope-2/run-3/foo.jsonl") or
// the absolute container path the pipeline writes to artifact_path
// ("/data/artifacts/program-1/scope-2/run-3/foo.jsonl") — when given the
// absolute form we strip the artifacts root prefix so the join stays sane.
func (h *InternalRunsHandler) Artifact(w http.ResponseWriter, r *http.Request) {
	rel := r.URL.Query().Get("path")
	if rel == "" {
		writeErr(w, http.StatusBadRequest, "path required")
		return
	}
	if strings.Contains(rel, "..") {
		writeErr(w, http.StatusBadRequest, "invalid path")
		return
	}
	rootAbs, _ := filepath.Abs(h.ArtifactsDir)
	relClean := filepath.Clean(rel)
	// If caller passed the absolute container path, peel the prefix so the
	// path we Join below resolves under ArtifactsDir.
	if absRel, err := filepath.Abs(relClean); err == nil && strings.HasPrefix(absRel, rootAbs) {
		if trimmed := strings.TrimPrefix(absRel, rootAbs); trimmed != "" {
			relClean = trimmed
		}
	}
	full := filepath.Join(h.ArtifactsDir, filepath.Clean("/"+relClean))
	abs, err := filepath.Abs(full)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if !strings.HasPrefix(abs, rootAbs) {
		writeErr(w, http.StatusForbidden, "escapes artifacts dir")
		return
	}
	http.ServeFile(w, r, abs)
}
