package jobs

import (
	"context"
	"encoding/json"
	"time"

	"github.com/hibiken/asynq"
)

const TypePipelineRun = "pipeline:run"

type PipelineRunPayload struct {
	RunID     int64  `json:"run_id"`
	ProgramID int64  `json:"program_id"`
	ScopeID   int64  `json:"scope_id"`
	ScopeVal  string `json:"scope_value"`
	ScopeKind string `json:"scope_kind"`
}

type Client struct {
	c *asynq.Client
}

func NewClient(redisAddr string) *Client {
	return &Client{
		c: asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr}),
	}
}

func (c *Client) Close() error {
	return c.c.Close()
}

func (c *Client) EnqueuePipeline(ctx context.Context, runID, programID, scopeID int64, value, kind string) error {
	p, err := json.Marshal(PipelineRunPayload{
		RunID:     runID,
		ProgramID: programID,
		ScopeID:   scopeID,
		ScopeVal:  value,
		ScopeKind: kind,
	})
	if err != nil {
		return err
	}
	t := asynq.NewTask(TypePipelineRun, p)
	_, err = c.c.EnqueueContext(ctx, t,
		asynq.Queue("pipeline"),
		asynq.MaxRetry(2),
		asynq.Timeout(4*time.Hour),
		asynq.Retention(24*time.Hour),
	)
	return err
}
