package handlers

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/go-github/v57/github"
	"github.com/issue-migrator/backend/models"
)

func GetGitHubIssues(c *gin.Context) {
	var req models.GitHubRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	client := github.NewClient(nil)
	if req.Token != "" {
		client = github.NewClient(nil).WithAuthToken(req.Token)
	}

	ctx := context.Background()
	opts := &github.IssueListByRepoOptions{
		State:       "all",
		ListOptions: github.ListOptions{PerPage: 100},
	}

	var allIssues []*github.Issue
	for {
		issues, resp, err := client.Issues.ListByRepo(ctx, req.Owner, req.Repo, opts)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		allIssues = append(allIssues, issues...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	var convertedIssues []models.Issue
	for _, issue := range allIssues {
		if issue.PullRequestLinks != nil {
			continue
		}
		convertedIssues = append(convertedIssues, models.ConvertGitHubIssue(issue))
	}

	c.JSON(http.StatusOK, gin.H{
		"issues": convertedIssues,
		"count":  len(convertedIssues),
	})
}