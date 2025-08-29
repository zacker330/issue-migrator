package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/issue-migrator/backend/models"
	"github.com/xanzy/go-gitlab"
)

func GetGitLabIssues(c *gin.Context) {
	var req models.GitLabRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	git, err := gitlab.NewClient(req.Token, gitlab.WithBaseURL(req.BaseURL))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	opts := &gitlab.ListProjectIssuesOptions{
		ListOptions: gitlab.ListOptions{
			PerPage: 100,
			Page:    1,
		},
	}

	var allIssues []*gitlab.Issue
	for {
		issues, resp, err := git.Issues.ListProjectIssues(req.ProjectID, opts)
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
		convertedIssues = append(convertedIssues, models.ConvertGitLabIssue(issue))
	}

	c.JSON(http.StatusOK, gin.H{
		"issues": convertedIssues,
		"count":  len(convertedIssues),
	})
}