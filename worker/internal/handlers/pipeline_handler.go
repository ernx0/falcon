package handlers

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/erhan/falcon/worker/internal/pipeline"
	"github.com/hibiken/asynq"
)

type PipelineHandler struct {
	Pipeline *pipeline.Pipeline
	Log      *slog.Logger
}

const TypePipelineRun = "pipeline:run"

type pipelinePayload struct {
	RunID      int64  `json:"run_id"`
	ProgramID  int64  `json:"program_id"`
	ScopeID   int64  `json:"scope_id"`
	ScopeVal  string `json:"scope_value"`
	ScopeKind string `json:"scope_kind"`
}

func (h *PipelineHandler) Handle(ctx context.Context, t *asynq.Task) error {
	var p pipelinePayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return err
	}
	h.Log.Info("pipeline received", "run", p.RunID, "target", p.ScopeVal)
	return h.Pipeline.Run(ctx, pipeline.Job{
		RunID:      p.RunID,
		ProgramID:  p.ProgramID,
		ScopeID:   p.ScopeID,
		ScopeVal:  p.ScopeVal,
		ScopeKind: p.ScopeKind,
	})
}
