package main

import (
	"log/slog"
	"os"

	"github.com/erhan/falcon/worker/internal/apiclient"
	"github.com/erhan/falcon/worker/internal/config"
	"github.com/erhan/falcon/worker/internal/handlers"
	"github.com/erhan/falcon/worker/internal/pipeline"
	"github.com/hibiken/asynq"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(cfg.ArtifactsDir, 0o755); err != nil {
		log.Error("artifacts dir", "err", err)
		os.Exit(1)
	}

	api := apiclient.New(cfg.APIURL, cfg.WorkerToken)
	pl := &pipeline.Pipeline{API: api, ArtifactsDir: cfg.ArtifactsDir, Log: log}
	h := &handlers.PipelineHandler{Pipeline: pl, Log: log}

	srv := asynq.NewServer(
		asynq.RedisClientOpt{Addr: cfg.RedisAddr},
		asynq.Config{
			Concurrency: 8,
			Queues: map[string]int{
				"pipeline": 6,
				"default":  2,
			},
			Logger: asynqLogger{log: log},
		},
	)

	mux := asynq.NewServeMux()
	mux.HandleFunc(handlers.TypePipelineRun, h.Handle)

	log.Info("worker starting", "redis", cfg.RedisAddr, "api", cfg.APIURL)
	if err := srv.Run(mux); err != nil {
		log.Error("asynq run", "err", err)
		os.Exit(1)
	}
}

type asynqLogger struct{ log *slog.Logger }

func (l asynqLogger) Debug(args ...interface{}) { l.log.Debug("asynq", "msg", args) }
func (l asynqLogger) Info(args ...interface{})  { l.log.Info("asynq", "msg", args) }
func (l asynqLogger) Warn(args ...interface{})  { l.log.Warn("asynq", "msg", args) }
func (l asynqLogger) Error(args ...interface{}) { l.log.Error("asynq", "msg", args) }
func (l asynqLogger) Fatal(args ...interface{}) { l.log.Error("asynq fatal", "msg", args); os.Exit(1) }
