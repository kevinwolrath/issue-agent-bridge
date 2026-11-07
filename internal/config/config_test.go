package config

import "testing"

func TestConfig(t *testing.T) {
	t.Setenv("GITHUB_WEBHOOK_SECRET", "test")
	t.Setenv("GITHUB_ALLOWED_REPOS", "Example/One,example/two")
	t.Setenv("WORKER_ENABLED", "")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.WorkerEnabled || len(c.Repositories) != 2 || !c.Repositories["example/one"] {
		t.Fatalf("unexpected config %+v", c)
	}
	for _, key := range []string{"GITHUB_WEBHOOK_SECRET", "GITHUB_ALLOWED_REPOS"} {
		t.Run(key, func(t *testing.T) {
			t.Setenv(key, "")
			if _, err := Load(); err == nil {
				t.Fatal("missing required config accepted")
			}
		})
	}
	t.Setenv("GITHUB_ALLOWED_REPOS", "*")
	if _, err := Load(); err == nil {
		t.Fatal("wildcard accepted")
	}
	t.Setenv("GITHUB_ALLOWED_REPOS", "example/one")
	t.Setenv("WORKER_ENABLED", "invalid")
	if _, err := Load(); err == nil {
		t.Fatal("invalid worker flag accepted")
	}
}
