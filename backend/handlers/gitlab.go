package handlers

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/issue-migrator/backend/models"
	"github.com/xanzy/go-gitlab"
)

func GetGitLabIssues(c *gin.Context) {
	fmt.Println("[GITLAB] GetGitLabIssues endpoint called")

	// Add panic recovery
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("[ERROR] Panic recovered in GetGitLabIssues: %v\n", r)
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Internal server error: %v", r)})
		}
	}()

	var req models.GitLabRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fmt.Printf("[ERROR] Failed to bind JSON: %v\n", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	fmt.Printf("[GITLAB] Request: ProjectID=%d, BaseURL=%s, Token=***\n", req.ProjectID, req.BaseURL)

	fmt.Println("[GITLAB] Creating GitLab client...")
	git, err := gitlab.NewClient(req.Token, gitlab.WithBaseURL(req.BaseURL))
	if err != nil {
		fmt.Printf("[ERROR] Failed to create GitLab client: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to create GitLab client: %v", err)})
		return
	}
	fmt.Println("[GITLAB] GitLab client created successfully")

	opts := &gitlab.ListProjectIssuesOptions{
		ListOptions: gitlab.ListOptions{
			PerPage: 100,
			Page:    1,
		},
	}

	fmt.Printf("[GITLAB] Fetching issues for project ID: %d\n", req.ProjectID)
	var allIssues []*gitlab.Issue
	for {
		fmt.Printf("[GITLAB] Fetching page %d...\n", opts.Page)
		issues, resp, err := git.Issues.ListProjectIssues(req.ProjectID, opts)
		if err != nil {
			fmt.Printf("[ERROR] Failed to list GitLab issues: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to fetch GitLab issues: %v", err)})
			return
		}
		fmt.Printf("[GITLAB] Fetched %d issues on page %d\n", len(issues), opts.Page)
		allIssues = append(allIssues, issues...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	fmt.Printf("[GITLAB] Converting %d issues...\n", len(allIssues))
	var convertedIssues []models.Issue
	for i, issue := range allIssues {
		fmt.Printf("[GITLAB] Converting issue %d/%d (IID: %d)\n", i+1, len(allIssues), issue.IID)
		converted := models.ConvertGitLabIssue(issue)
		convertedIssues = append(convertedIssues, converted)
	}
	fmt.Printf("[GITLAB] Successfully converted %d issues\n", len(convertedIssues))

	fmt.Printf("[GITLAB] Returning %d issues to client\n", len(convertedIssues))
	c.JSON(http.StatusOK, gin.H{
		"issues": convertedIssues,
		"count":  len(convertedIssues),
	})
}