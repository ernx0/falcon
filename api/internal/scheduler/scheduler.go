package scheduler

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/erhan/falcon/api/internal/jobs"
	"github.com/jmoiron/sqlx"
	"github.com/robfig/cron/v3"
)

const advisoryLockKey int64 = 84329110 // arbitrary; "Falcon scheduler"

type Scheduler struct {
	DB     *sqlx.DB
	Jobs   *jobs.Client
	Parser cron.Parser
	Log    *slog.Logger
}

func New(db *sqlx.DB, j *jobs.Client, log *slog.Logger) *Scheduler {
	return &Scheduler{
		DB:     db,
		Jobs:   j,
		Parser: cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow),
		Log:    log,
	}
}

// Start runs the scheduler tick every minute. Uses pg_advisory_lock so only
// one API replica processes due targets.
func (s *Scheduler) Start(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	// First run shortly after boot.
	time.AfterFunc(5*time.Second, func() { s.tick(ctx) })
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

func (s *Scheduler) tick(ctx context.Context) {
	conn, err := s.DB.Connx(ctx)
	if err != nil {
		s.Log.Warn("scheduler conn failed", "err", err)
		return
	}
	defer conn.Close()

	var locked bool
	if err := conn.GetContext(ctx, &locked, `SELECT pg_try_advisory_lock($1)`, advisoryLockKey); err != nil || !locked {
		return
	}
	defer conn.ExecContext(ctx, `SELECT pg_advisory_unlock($1)`, advisoryLockKey)

	rows, err := conn.QueryxContext(ctx, `
SELECT id, program_id, value, kind, schedule_cron
FROM scope
WHERE enabled = TRUE
  AND schedule_cron IS NOT NULL
  AND (next_run_at IS NULL OR next_run_at <= NOW())`)
	if err != nil {
		s.Log.Warn("scheduler query failed", "err", err)
		return
	}
	defer rows.Close()

	type due struct {
		ID        int64
		ProgramID int64
		Value     string
		Kind      string
		Cron      string
	}
	var todo []due
	for rows.Next() {
		var d due
		var cronVal sql.NullString
		if err := rows.Scan(&d.ID, &d.ProgramID, &d.Value, &d.Kind, &cronVal); err != nil {
			continue
		}
		if !cronVal.Valid {
			continue
		}
		d.Cron = cronVal.String
		todo = append(todo, d)
	}

	for _, d := range todo {
		sched, err := s.Parser.Parse(d.Cron)
		if err != nil {
			s.Log.Warn("invalid cron", "target", d.ID, "cron", d.Cron, "err", err)
			continue
		}
		next := sched.Next(time.Now())

		var runID int64
		err = conn.GetContext(ctx, &runID, `
INSERT INTO runs(scope_id, program_id, trigger, status) VALUES($1,$2,'schedule','queued')
RETURNING id`, d.ID, d.ProgramID)
		if err != nil {
			s.Log.Warn("create run failed", "target", d.ID, "err", err)
			continue
		}

		if _, err := conn.ExecContext(ctx,
			`UPDATE scope SET next_run_at=$2 WHERE id=$1`, d.ID, next); err != nil {
			s.Log.Warn("update next_run failed", "target", d.ID, "err", err)
		}

		if err := s.Jobs.EnqueuePipeline(ctx, runID, d.ProgramID, d.ID, d.Value, d.Kind); err != nil {
			s.Log.Warn("enqueue failed", "run", runID, "err", err)
			conn.ExecContext(ctx,
				`UPDATE runs SET status='failed', error=$2, finished_at=NOW() WHERE id=$1`,
				runID, err.Error())
		} else {
			s.Log.Info("scheduled run", "run", runID, "target", d.ID, "next", next)
		}
	}
}
