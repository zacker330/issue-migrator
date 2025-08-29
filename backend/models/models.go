package models

import (
	"time"

	"github.com/google/go-github/v57/github"
	"github.com/xanzy/go-gitlab"
)

type Issue struct {
	ID          int       `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	State       string    `json:"state"`
	Labels      []string  `json:"labels"`
	Author      string    `json:"author"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	URL         string    `json:"url"`
}

type GitHubRequest struct {
	Owner string `json:"owner" binding:"required"`
	Repo  string `json:"repo" binding:"required"`
	Token string `json:"token"`
}

type GitLabRequest struct {
	BaseURL   string `json:"base_url" binding:"required"`
	ProjectID int    `json:"project_id" binding:"required"`
	Token     string `json:"token" binding:"required"`
}

type MigrationRequest struct {
	Direction string `json:"direction" binding:"required"`
	Source    struct {
		Type      string `json:"type"`
		Owner     string `json:"owner"`
		Repo      string `json:"repo"`
		ProjectID int    `json:"project_id"`
		BaseURL   string `json:"base_url"`
		Token     string `json:"token"`
	} `json:"source" binding:"required"`
	Target struct {
		Type      string `json:"type"`
		Owner     string `json:"owner"`
		Repo      string `json:"repo"`
		ProjectID int    `json:"project_id"`
		BaseURL   string `json:"base_url"`
		Token     string `json:"token"`
	} `json:"target" binding:"required"`
	IssueIDs []int `json:"issue_ids" binding:"required"`
}

type MigrationStatus struct {
	OriginalID int    `json:"original_id"`
	NewID      int    `json:"new_id"`
	NewURL     string `json:"new_url"`
	Error      string `json:"error,omitempty"`
}

type MigrationResult struct {
	Success []MigrationStatus `json:"success"`
	Failed  []MigrationStatus `json:"failed"`
}

func ConvertGitHubIssue(issue *github.Issue) Issue {
	labels := make([]string, len(issue.Labels))
	for i, label := range issue.Labels {
		labels[i] = label.GetName()
	}

	return Issue{
		ID:          issue.GetNumber(),
		Title:       issue.GetTitle(),
		Description: issue.GetBody(),
		State:       issue.GetState(),
		Labels:      labels,
		Author:      issue.User.GetLogin(),
		CreatedAt:   issue.GetCreatedAt().Time,
		UpdatedAt:   issue.GetUpdatedAt().Time,
		URL:         issue.GetHTMLURL(),
	}
}

func ConvertGitLabIssue(issue *gitlab.Issue) Issue {
	return Issue{
		ID:          issue.IID,
		Title:       issue.Title,
		Description: issue.Description,
		State:       issue.State,
		Labels:      issue.Labels,
		Author:      issue.Author.Username,
		CreatedAt:   *issue.CreatedAt,
		UpdatedAt:   *issue.UpdatedAt,
		URL:         issue.WebURL,
	}
}