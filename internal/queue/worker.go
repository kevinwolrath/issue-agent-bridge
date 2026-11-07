package queue

import (
	"context"
	"log/slog"
	"time"

	"github.com/kevinwolrath/issue-agent-bridge/internal/agent"
	"github.com/kevinwolrath/issue-agent-bridge/internal/job"
)

type Queue interface {
	Claim(context.Context) (*job.Job, error)
	Finish(context.Context, int64, error) error
}

type Worker struct {
	Queue    Queue
	Agent    agent.Adapter
	Logger   *slog.Logger
	Interval time.Duration
}

func (w Worker) Step(ctx context.Context) (bool, error) {
	j, err := w.Queue.Claim(ctx)
	if err != nil || j == nil {
		return false, err
	}
	executionError := w.Agent.Execute(ctx, *j)
	// Persist even if shutdown canceled execution.
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := w.Queue.Finish(finishCtx, j.ID, executionError); err != nil {
		return true, err
	}
	w.Logger.Info("job transitioned", "job_id", j.ID, "success", executionError == nil)
	return true, nil
}

func (w Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.Interval)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		worked, err := w.Step(ctx)
		if err != nil {
			w.Logger.Error("worker step failed", "error", err)
		}
		if worked && err == nil {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
