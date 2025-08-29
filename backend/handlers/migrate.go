package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/go-github/v57/github"
	"github.com/issue-migrator/backend/models"
	"github.com/xanzy/go-gitlab"
)

func MigrateIssues(c *gin.Context) {
	var req models.MigrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	results := models.MigrationResult{
		Success: []models.MigrationStatus{},
		Failed:  []models.MigrationStatus{},
	}

	switch req.Direction {
	case "github-to-gitlab":
		results = migrateGitHubToGitLab(req)
	case "gitlab-to-github":
		results = migrateGitLabToGitHub(req)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid migration direction"})
		return
	}

	c.JSON(http.StatusOK, results)
}

func migrateGitHubToGitLab(req models.MigrationRequest) models.MigrationResult {
	result := models.MigrationResult{
		Success: []models.MigrationStatus{},
		Failed:  []models.MigrationStatus{},
	}

	ghClient := github.NewClient(nil).WithAuthToken(req.Source.Token)
	glClient, _ := gitlab.NewClient(req.Target.Token, gitlab.WithBaseURL(req.Target.BaseURL))

	ctx := context.Background()

	for _, issueID := range req.IssueIDs {
		issue, _, err := ghClient.Issues.Get(ctx, req.Source.Owner, req.Source.Repo, issueID)
		if err != nil {
			result.Failed = append(result.Failed, models.MigrationStatus{
				OriginalID: issueID,
				Error:      err.Error(),
			})
			continue
		}

		labels := make([]string, len(issue.Labels))
		for i, label := range issue.Labels {
			labels[i] = label.GetName()
		}

		description := fmt.Sprintf("*Migrated from GitHub: %s*\n\n%s", issue.GetHTMLURL(), issue.GetBody())
		title := issue.GetTitle()

		createOpts := &gitlab.CreateIssueOptions{
			Title:       &title,
			Description: &description,
			Labels:      (*gitlab.Labels)(&labels),
		}

		newIssue, _, err := glClient.Issues.CreateIssue(req.Target.ProjectID, createOpts)
		if err != nil {
			result.Failed = append(result.Failed, models.MigrationStatus{
				OriginalID: issueID,
				Error:      err.Error(),
			})
			continue
		}

		comments, _, err := ghClient.Issues.ListComments(ctx, req.Source.Owner, req.Source.Repo, issueID, nil)
		if err == nil {
			for _, comment := range comments {
				body := fmt.Sprintf("**@%s commented:**\n\n%s", comment.User.GetLogin(), comment.GetBody())
				noteOpts := &gitlab.CreateIssueNoteOptions{
					Body: &body,
				}
				glClient.Notes.CreateIssueNote(req.Target.ProjectID, newIssue.IID, noteOpts)
			}
		}

		result.Success = append(result.Success, models.MigrationStatus{
			OriginalID: issueID,
			NewID:      newIssue.IID,
			NewURL:     newIssue.WebURL,
		})
	}

	return result
}

func migrateGitLabToGitHub(req models.MigrationRequest) models.MigrationResult {
	result := models.MigrationResult{
		Success: []models.MigrationStatus{},
		Failed:  []models.MigrationStatus{},
	}

	glClient, _ := gitlab.NewClient(req.Source.Token, gitlab.WithBaseURL(req.Source.BaseURL))
	ghClient := github.NewClient(nil).WithAuthToken(req.Target.Token)

	ctx := context.Background()

	for _, issueID := range req.IssueIDs {
		issue, _, err := glClient.Issues.GetIssue(req.Source.ProjectID, issueID)
		if err != nil {
			result.Failed = append(result.Failed, models.MigrationStatus{
				OriginalID: issueID,
				Error:      err.Error(),
			})
			continue
		}

		body := fmt.Sprintf("*Migrated from GitLab: %s*\n\n%s", issue.WebURL, issue.Description)

		labels := make([]string, len(issue.Labels))
		for i, label := range issue.Labels {
			labels[i] = label
		}

		createReq := &github.IssueRequest{
			Title:  &issue.Title,
			Body:   &body,
			Labels: &labels,
		}

		if strings.ToLower(issue.State) == "closed" {
			state := "closed"
			createReq.State = &state
		}

		newIssue, _, err := ghClient.Issues.Create(ctx, req.Target.Owner, req.Target.Repo, createReq)
		if err != nil {
			result.Failed = append(result.Failed, models.MigrationStatus{
				OriginalID: issueID,
				Error:      err.Error(),
			})
			continue
		}

		notes, _, err := glClient.Notes.ListIssueNotes(req.Source.ProjectID, issueID, nil)
		if err == nil {
			for _, note := range notes {
				body := fmt.Sprintf("**@%s commented:**\n\n%s", note.Author.Username, note.Body)
				comment := &github.IssueComment{
					Body: &body,
				}
				ghClient.Issues.CreateComment(ctx, req.Target.Owner, req.Target.Repo, newIssue.GetNumber(), comment)
			}
		}

		result.Success = append(result.Success, models.MigrationStatus{
			OriginalID: issueID,
			NewID:      newIssue.GetNumber(),
			NewURL:     newIssue.GetHTMLURL(),
		})
	}

	return result
}