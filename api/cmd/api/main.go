package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/erhan/falcon/api/internal/auth"
	"github.com/erhan/falcon/api/internal/config"
	"github.com/erhan/falcon/api/internal/db"
	"github.com/erhan/falcon/api/internal/handlers"
	"github.com/erhan/falcon/api/internal/jobs"
	"github.com/erhan/falcon/api/internal/scheduler"
	"github.com/erhan/falcon/api/internal/scope"
	"github.com/erhan/falcon/api/internal/server"

	_ "github.com/erhan/falcon/api/docs"
)

// @title           Falcon API
// @version         0.1.0
// @description     Bug bounty & continuous recon backend.
// @BasePath        /
//
// @securityDefinitions.apikey  BearerAuth
// @in                          header
// @name                        Authorization
// @description                 JWT token issued by /api/auth/login. Format: "Bearer {token}".

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}

	if err := db.Migrate(cfg.DBDSN, cfg.MigrationsDir); err != nil {
		log.Error("migrate", "err", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	pool, err := db.Open(ctx, cfg.DBDSN)
	if err != nil {
		log.Error("db open", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := seedAdmin(ctx, pool, cfg.AdminEmail, cfg.AdminPassword); err != nil {
		log.Error("seed admin", "err", err)
		os.Exit(1)
	}

	// Realign every existing report's severity with its current VRT id —
	// covers data written before severity became a derived field.
	if n, err := handlers.RecomputeAllReportSeverities(ctx, pool); err != nil {
		log.Warn("recompute severities", "err", err)
	} else if n > 0 {
		log.Info("recompute severities", "updated", n)
	}

	authSvc := auth.New(cfg.JWTSecret, cfg.WorkerToken)
	jobsClient := jobs.NewClient(cfg.RedisAddr)
	defer jobsClient.Close()
	scopeCache := scope.NewCache()

	sched := scheduler.New(pool, jobsClient, log)
	go sched.Start(ctx)

	handler := server.New(server.Deps{
		DB:           pool,
		Auth:         authSvc,
		Jobs:         jobsClient,
		ScopeCache:   scopeCache,
		ArtifactsDir: cfg.ArtifactsDir,
	})

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           handler,
		ReadHeaderTimeout: 15 * time.Second,
	}
	log.Info("api listening", "addr", cfg.ListenAddr)

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("listen", "err", err)
			cancel()
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")
	shutCtx, sc := context.WithTimeout(context.Background(), 10*time.Second)
	defer sc()
	_ = srv.Shutdown(shutCtx)
}
