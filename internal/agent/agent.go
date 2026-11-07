// Package agent defines the future coding agent boundary. Only Mock is provided.
package agent

import (
	"context"
	"log/slog"

	"github.com/kevinwolrath/issue-agent-bridge/internal/job"
)

type Adapter interface {
	Execute(context.Context, job.Job) error
}

type Mock struct{ Logger *slog.Logger }

func (m Mock) Execute(ctx context.Context, j job.Job) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.Logger.Info("mock job completed", "job_id", j.ID, "repository", j.Repository, "issue_number", j.IssueNumber)
	return nil
}
