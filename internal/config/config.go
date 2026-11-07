package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Address       string
	DatabasePath  string
	WebhookSecret string
	ReadyLabel    string
	Repositories  map[string]bool
	WorkerEnabled bool
}

func Load() (Config, error) {
	c := Config{Address: value("HTTP_ADDR", ":8080"), DatabasePath: value("DATABASE_PATH", "bridge.db"), WebhookSecret: os.Getenv("GITHUB_WEBHOOK_SECRET"), ReadyLabel: value("GITHUB_READY_LABEL", "agent:ready"), Repositories: map[string]bool{}}
	if strings.TrimSpace(c.WebhookSecret) == "" {
		return c, errors.New("GITHUB_WEBHOOK_SECRET is required")
	}
	for _, repo := range strings.Split(os.Getenv("GITHUB_ALLOWED_REPOS"), ",") {
		repo = strings.ToLower(strings.TrimSpace(repo))
		parts := strings.Split(repo, "/")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" || strings.ContainsAny(repo, " *?\\\t\r\n") {
			return c, fmt.Errorf("invalid GITHUB_ALLOWED_REPOS entry %q; use owner/repo", repo)
		}
		c.Repositories[repo] = true
	}
	var err error
	c.WorkerEnabled, err = strconv.ParseBool(value("WORKER_ENABLED", "false"))
	if err != nil {
		return c, errors.New("WORKER_ENABLED must be true or false")
	}
	return c, nil
}

func value(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
