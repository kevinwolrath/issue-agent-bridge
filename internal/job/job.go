// Package job defines the durable work contract shared by providers and agents.
package job

import "time"

type Input struct {
	Provider    string `json:"provider"`
	DeliveryID  string `json:"delivery_id"`
	Repository  string `json:"repository"`
	IssueNumber int64  `json:"issue_number"`
	IssueURL    string `json:"issue_url"`
}

type Job struct {
	Input
	ID        int64     `json:"id"`
	Status    string    `json:"status"`
	Attempts  int       `json:"attempts"`
	LastError string    `json:"last_error"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
