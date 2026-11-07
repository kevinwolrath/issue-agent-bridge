// Package provider defines the boundary for future issue tracker adapters.
package provider

import "github.com/kevinwolrath/issue-agent-bridge/internal/job"

type Adapter interface {
	Parse(event string, body []byte) (*job.Input, error)
}
