package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/kevinwolrath/issue-agent-bridge/internal/agent"
	"github.com/kevinwolrath/issue-agent-bridge/internal/config"
	"github.com/kevinwolrath/issue-agent-bridge/internal/httpapi"
	"github.com/kevinwolrath/issue-agent-bridge/internal/provider"
	"github.com/kevinwolrath/issue-agent-bridge/internal/queue"
	"github.com/kevinwolrath/issue-agent-bridge/internal/store"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("service stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	c, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	startup, cancelStartup := context.WithTimeout(ctx, 15*time.Second)
	db, err := store.Open(startup, c.DatabasePath)
	if err == nil {
		err = db.Recover(startup)
	}
	cancelStartup()
	if db != nil {
		defer db.Close()
	}
	if err != nil {
		return err
	}
	api := httpapi.Server{Store: db, Provider: provider.GitHub{ReadyLabel: c.ReadyLabel, Repositories: c.Repositories}, Secret: []byte(c.WebhookSecret), Logger: logger}
	server := &http.Server{Addr: c.Address, Handler: api.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	workerCtx, cancelWorker := context.WithCancel(ctx)
	defer cancelWorker()
	var workers sync.WaitGroup
	if c.WorkerEnabled {
		workers.Add(1)
		go func() {
			defer workers.Done()
			(queue.Worker{Queue: db, Agent: agent.Mock{Logger: logger}, Logger: logger, Interval: time.Second}).Run(workerCtx)
		}()
	}
	done := make(chan error, 1)
	go func() { done <- server.ListenAndServe() }()
	logger.Info("service started", "address", c.Address, "worker_enabled", c.WorkerEnabled, "agent", "mock")
	select {
	case <-ctx.Done():
	case err = <-done:
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
	}
	cancelWorker()
	shutdown, cancelShutdown := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancelShutdown()
	if shutdownErr := server.Shutdown(shutdown); shutdownErr != nil {
		_ = server.Close()
		if err == nil {
			err = shutdownErr
		}
	}
	workers.Wait()
	logger.Info("service stopped")
	return err
}
