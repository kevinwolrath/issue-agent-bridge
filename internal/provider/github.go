package provider

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kevinwolrath/issue-agent-bridge/internal/job"
)

// GitHub accepts only issues.labeled events for an open issue and the exact
// configured label. Adding an existing label to a different event is not a trigger.
type GitHub struct {
	ReadyLabel   string
	Repositories map[string]bool
}

func (g GitHub) Parse(event string, body []byte) (*job.Input, error) {
	if event != "issues" {
		return nil, nil
	}
	var e struct {
		Action string `json:"action"`
		Label  struct {
			Name string `json:"name"`
		} `json:"label"`
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
		Issue struct {
			Number      int64           `json:"number"`
			State       string          `json:"state"`
			HTMLURL     string          `json:"html_url"`
			PullRequest json.RawMessage `json:"pull_request"`
		} `json:"issue"`
	}
	if err := json.Unmarshal(body, &e); err != nil {
		return nil, fmt.Errorf("decode GitHub event: %w", err)
	}
	if e.Action != "labeled" || e.Label.Name != g.ReadyLabel || e.Issue.State != "open" ||
		!g.Repositories[strings.ToLower(e.Repository.FullName)] || e.Issue.Number <= 0 ||
		(len(e.Issue.PullRequest) > 0 && string(e.Issue.PullRequest) != "null") {
		return nil, nil
	}
	return &job.Input{Provider: "github", Repository: strings.ToLower(e.Repository.FullName), IssueNumber: e.Issue.Number, IssueURL: e.Issue.HTMLURL}, nil
}
