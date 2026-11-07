package store_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/kevinwolrath/issue-agent-bridge/internal/job"
	"github.com/kevinwolrath/issue-agent-bridge/internal/store"
)

func TestPersistenceAndTransitions(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "jobs.db")
	db, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	in := job.Input{Provider: "github", DeliveryID: "delivery-1", Repository: "example/project", IssueNumber: 1}
	created, err := db.Enqueue(ctx, in)
	if err != nil || !created {
		t.Fatalf("enqueue %v %v", created, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	created, err = db.Enqueue(ctx, in)
	if err != nil || created {
		t.Fatalf("duplicate after restart %v %v", created, err)
	}
	j, err := db.Claim(ctx)
	if err != nil || j == nil {
		t.Fatalf("claim %v %v", j, err)
	}
	if j.Status != "running" || j.Attempts != 1 || j.CreatedAt.IsZero() || j.UpdatedAt.Before(j.CreatedAt) {
		t.Fatalf("bad claim %+v", j)
	}
	if err := db.Finish(ctx, j.ID, nil); err != nil {
		t.Fatal(err)
	}
	got, err := db.Get(ctx, j.ID)
	if err != nil || got.Status != "succeeded" {
		t.Fatalf("finish %+v %v", got, err)
	}
	if err := db.Finish(ctx, j.ID, nil); err == nil {
		t.Fatal("illegal transition accepted")
	}
	in.DeliveryID = "delivery-2"
	if _, err := db.Enqueue(ctx, in); err != nil {
		t.Fatal(err)
	}
	j, err = db.Claim(ctx)
	if err != nil || j == nil {
		t.Fatal(err)
	}
	if err := db.Finish(ctx, j.ID, errors.New("mock failure")); err != nil {
		t.Fatal(err)
	}
	got, err = db.Get(ctx, j.ID)
	if err != nil || got.Status != "failed" || got.LastError != "mock failure" {
		t.Fatalf("failure %+v %v", got, err)
	}
	in.DeliveryID = "delivery-3"
	if _, err := db.Enqueue(ctx, in); err != nil {
		t.Fatal(err)
	}
	j, err = db.Claim(ctx)
	if err != nil || j == nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	got, err = db.Get(ctx, j.ID)
	if err != nil || got.Status != "failed" || got.LastError == "" {
		t.Fatalf("recovery %+v %v", got, err)
	}
	if next, err := db.Claim(ctx); err != nil || next != nil {
		t.Fatalf("terminal jobs reclaimed %+v %v", next, err)
	}
}

func TestConcurrentDeliveryAndClaims(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "jobs.db")
	a, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			db := a
			if i%2 == 0 {
				db = b
			}
			if _, err := db.Enqueue(ctx, job.Input{Provider: "github", DeliveryID: "same", Repository: "example/project", IssueNumber: 1}); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	claimed := make(chan int64, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			db := a
			if i%2 == 0 {
				db = b
			}
			j, err := db.Claim(ctx)
			if err != nil {
				t.Error(err)
			}
			if j != nil {
				claimed <- j.ID
			}
		}(i)
	}
	wg.Wait()
	close(claimed)
	if len(claimed) != 1 {
		t.Fatalf("claimed %d times", len(claimed))
	}
}
