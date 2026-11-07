package queue_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/kevinwolrath/issue-agent-bridge/internal/agent"
	"github.com/kevinwolrath/issue-agent-bridge/internal/job"
	"github.com/kevinwolrath/issue-agent-bridge/internal/queue"
	"github.com/kevinwolrath/issue-agent-bridge/internal/store"
)

type failingAgent struct{}

func (failingAgent) Execute(context.Context, job.Job) error { return errors.New("mock failure") }

func TestWorker(t *testing.T) {
	ctx := context.Background()
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	for _, tc := range []struct {
		name, status string
		adapter      agent.Adapter
	}{{"mock", "succeeded", agent.Mock{Logger: log}}, {"failure", "failed", failingAgent{}}} {
		t.Run(tc.name, func(t *testing.T) {
			db, err := store.Open(ctx, filepath.Join(t.TempDir(), "jobs.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if _, err := db.Enqueue(ctx, job.Input{Provider: "github", DeliveryID: "one", Repository: "example/project", IssueNumber: 1}); err != nil {
				t.Fatal(err)
			}
			w := queue.Worker{Queue: db, Agent: tc.adapter, Logger: log}
			if worked, err := w.Step(ctx); err != nil || !worked {
				t.Fatalf("step %v %v", worked, err)
			}
			j, err := db.Get(ctx, 1)
			if err != nil || j.Status != tc.status || j.Attempts != 1 {
				t.Fatalf("job %+v %v", j, err)
			}
			if worked, err := w.Step(ctx); err != nil || worked {
				t.Fatalf("empty step %v %v", worked, err)
			}
		})
	}
}
