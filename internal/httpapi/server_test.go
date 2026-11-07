package httpapi_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kevinwolrath/issue-agent-bridge/internal/httpapi"
	"github.com/kevinwolrath/issue-agent-bridge/internal/provider"
	"github.com/kevinwolrath/issue-agent-bridge/internal/store"
)

const payload = `{"action":"labeled","label":{"name":"agent:ready"},"repository":{"full_name":"example/project"},"issue":{"number":42,"state":"open","html_url":"https://github.com/example/project/issues/42"}}`

func signature(body []byte) string {
	m := hmac.New(sha256.New, []byte("test-secret"))
	m.Write(body)
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

func TestSignature(t *testing.T) {
	body := []byte(payload)
	for _, tc := range []struct {
		name, sig    string
		secret, body []byte
		want         bool
	}{
		{"valid", signature(body), []byte("test-secret"), body, true},
		{"tampered", signature(body), []byte("test-secret"), append(append([]byte{}, body...), ' '), false},
		{"wrong secret", signature(body), []byte("wrong"), body, false},
		{"empty secret", signature(body), nil, body, false},
		{"missing", "", []byte("test-secret"), body, false},
		{"malformed", "sha256=xyz", []byte("test-secret"), body, false},
		{"wrong algorithm", "sha1=abc", []byte("test-secret"), body, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := httpapi.ValidSignature(tc.secret, tc.body, tc.sig); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestWebhook(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := (httpapi.Server{Store: db, Provider: provider.GitHub{ReadyLabel: "agent:ready", Repositories: map[string]bool{"example/project": true}}, Secret: []byte("test-secret"), Logger: slog.New(slog.NewJSONHandler(io.Discard, nil)), MaxBodyBytes: 1024}).Handler()
	for i, tc := range []struct {
		name, body, event    string
		status               int
		unsigned, noDelivery bool
	}{
		{"eligible", payload, "issues", 202, false, false},
		{"duplicate", payload, "issues", 200, false, false},
		{"unapproved repository", strings.ReplaceAll(payload, "example/project", "other/project"), "issues", 204, false, false},
		{"wrong label", strings.ReplaceAll(payload, "agent:ready", "other"), "issues", 204, false, false},
		{"opened", strings.ReplaceAll(payload, "labeled", "opened"), "issues", 204, false, false},
		{"closed issue", strings.ReplaceAll(payload, `"open"`, `"closed"`), "issues", 204, false, false},
		{"pull request", strings.ReplaceAll(payload, `"number":42`, `"number":42,"pull_request":{}`), "issues", 204, false, false},
		{"irrelevant event", payload, "push", 204, false, false},
		{"missing signature", payload, "issues", 401, true, false},
		{"missing delivery", payload, "issues", 400, false, true},
		{"malformed JSON", "{", "issues", 400, false, false},
		{"body limit", strings.Repeat("x", 1025), "issues", 413, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/webhooks/github", bytes.NewBufferString(tc.body))
			if !tc.unsigned {
				r.Header.Set("X-Hub-Signature-256", signature([]byte(tc.body)))
			}
			delivery := fmt.Sprintf("delivery-%d", i)
			if i < 2 {
				delivery = "original"
			}
			if !tc.noDelivery {
				r.Header.Set("X-GitHub-Delivery", delivery)
			}
			r.Header.Set("X-GitHub-Event", tc.event)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("status %d want %d: %s", w.Code, tc.status, w.Body)
			}
		})
	}
	j, err := db.Claim(ctx)
	if err != nil || j == nil {
		t.Fatalf("missing job: %v", err)
	}
	if j.DeliveryID != "original" || j.IssueNumber != 42 {
		t.Fatalf("unexpected job %+v", j)
	}
	if j, err = db.Claim(ctx); err != nil || j != nil {
		t.Fatalf("unwanted enqueue: %+v %v", j, err)
	}
	for _, path := range []string{"/healthz", "/readyz"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
	db.Close()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/readyz", nil))
	if w.Code != 503 {
		t.Fatalf("closed store readiness: %d", w.Code)
	}
}
