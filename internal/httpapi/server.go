package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/kevinwolrath/issue-agent-bridge/internal/job"
	"github.com/kevinwolrath/issue-agent-bridge/internal/provider"
)

type Store interface {
	Enqueue(context.Context, job.Input) (bool, error)
	Ping(context.Context) error
}

type Server struct {
	Store        Store
	Provider     provider.Adapter
	Secret       []byte
	Logger       *slog.Logger
	MaxBodyBytes int64
}

func (s Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok\n")
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := s.Store.Ping(ctx); err != nil {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, "ready\n")
	})
	mux.HandleFunc("POST /webhooks/github", s.webhook)
	return mux
}

func ValidSignature(secret, body []byte, signature string) bool {
	if len(secret) == 0 || !strings.HasPrefix(signature, "sha256=") {
		return false
	}
	provided, err := hex.DecodeString(strings.TrimPrefix(signature, "sha256="))
	if err != nil || len(provided) != sha256.Size {
		return false
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(body)
	return hmac.Equal(provided, mac.Sum(nil))
}

func (s Server) webhook(w http.ResponseWriter, r *http.Request) {
	limit := s.MaxBodyBytes
	if limit <= 0 {
		limit = 1 << 20
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
		} else {
			http.Error(w, "cannot read body", http.StatusBadRequest)
		}
		return
	}
	if !ValidSignature(s.Secret, body, r.Header.Get("X-Hub-Signature-256")) {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}
	delivery := r.Header.Get("X-GitHub-Delivery")
	if strings.TrimSpace(delivery) == "" || len(delivery) > 128 {
		http.Error(w, "invalid delivery ID", http.StatusBadRequest)
		return
	}
	in, err := s.Provider.Parse(r.Header.Get("X-GitHub-Event"), body)
	if err != nil {
		http.Error(w, "invalid event", http.StatusBadRequest)
		return
	}
	if in == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	in.DeliveryID = delivery
	created, err := s.Store.Enqueue(r.Context(), *in)
	if err != nil {
		s.Logger.Error("enqueue failed", "delivery_id", delivery, "error", err)
		http.Error(w, "queue unavailable", http.StatusServiceUnavailable)
		return
	}
	s.Logger.Info("webhook accepted", "delivery_id", delivery, "created", created, "repository", in.Repository, "issue_number", in.IssueNumber)
	if created {
		w.WriteHeader(http.StatusAccepted)
	} else {
		w.WriteHeader(http.StatusOK)
	}
}
