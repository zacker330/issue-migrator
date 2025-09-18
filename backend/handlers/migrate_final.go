package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"mime/multipart"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/go-github/v57/github"
	"github.com/issue-migrator/backend/models"
	"github.com/xanzy/go-gitlab"
)

// MigrateIssuesFinal handles migration with proper image transfer and logging
func MigrateIssuesFinal(c *gin.Context) {
	var req models.MigrationRequest
	log.Println("这是一条普通日志信息") // 打印日志并换行
	fmt.Printf("[MIGRATE] Starting migration: ssssss\n")
	slog.Info("xxxx222222233333332xxxxx")

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Log migration request
	fmt.Printf("[MIGRATE] Starting migration: %s\n", req.Direction)
	fmt.Printf("[MIGRATE] Source: %+v\n", req.Source)
	fmt.Printf("[MIGRATE] Target: %+v\n", req.Target)
	fmt.Printf("[MIGRATE] Issues to migrate: %v\n", req.IssueIDs)
	slog.Info("xxxx222222444444xxxx", req.Direction)
	log.Println("log println")

	results := models.MigrationResult{
		Success: []models.MigrationStatus{},
		Failed:  []models.MigrationStatus{},
	}

	switch req.Direction {
	case "github-to-gitlab":
		results = migrateGHtoGLFinal(req, c)
	case "gitlab-to-github":
		results = migrateGLtoGHFinal(req, c)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid migration direction"})
		return
	}

	fmt.Printf("[MIGRATE] Migration completed. Success: %d, Failed: %d\n",
		len(results.Success), len(results.Failed))

	c.JSON(http.StatusOK, results)
}

func migrateGHtoGLFinal(req models.MigrationRequest, ginCtx *gin.Context) models.MigrationResult {
	result := models.MigrationResult{
		Success: []models.MigrationStatus{},
		Failed:  []models.MigrationStatus{},
	}

	ghClient := github.NewClient(nil).WithAuthToken(req.Source.Token)
	glClient, _ := gitlab.NewClient(req.Target.Token, gitlab.WithBaseURL(req.Target.BaseURL))
	ctx := context.Background()

	for _, issueID := range req.IssueIDs {
		fmt.Printf("[MIGRATE] Processing GitHub issue #%d\n", issueID)

		issue, _, err := ghClient.Issues.Get(ctx, req.Source.Owner, req.Source.Repo, issueID)
		if err != nil {
			fmt.Printf("[ERROR] Failed to fetch issue #%d: %v\n", issueID, err)
			result.Failed = append(result.Failed, models.MigrationStatus{
				OriginalID: issueID,
				Error:      err.Error(),
			})
			continue
		}

		// Process images in issue body
		fmt.Printf("[MIGRATE] Processing images in issue #%d body\n", issueID)
		processedBody := processImagesWithLogging(
			issue.GetBody(),
			req.Target.ProjectID,
			req.Target.Token,
			req.Target.BaseURL,
			req.Source.Token, // Pass GitHub token for authenticated download
		)

		labels := make([]string, len(issue.Labels))
		for i, label := range issue.Labels {
			labels[i] = label.GetName()
		}

		// Create migration header with timestamp information
		migrationHeader := fmt.Sprintf("### 🔄 Migrated from GitHub\n\n")
		migrationHeader += fmt.Sprintf("**Original Issue:** %s\n", issue.GetHTMLURL())
		migrationHeader += fmt.Sprintf("**Original Author:** @%s\n", issue.User.GetLogin())
		migrationHeader += fmt.Sprintf("**Created:** %s\n", issue.GetCreatedAt().Format("2006-01-02 15:04:05 UTC"))
		migrationHeader += fmt.Sprintf("**Last Updated:** %s\n", issue.GetUpdatedAt().Format("2006-01-02 15:04:05 UTC"))
		if issue.GetState() == "closed" && issue.ClosedAt != nil {
			migrationHeader += fmt.Sprintf("**Closed:** %s\n", issue.GetClosedAt().Format("2006-01-02 15:04:05 UTC"))
		}
		migrationHeader += fmt.Sprintf("**State:** %s\n\n", issue.GetState())
		migrationHeader += "---\n\n"

		description := migrationHeader + processedBody
		title := issue.GetTitle()

		createOpts := &gitlab.CreateIssueOptions{
			Title:       &title,
			Description: &description,
			Labels:      (*gitlab.Labels)(&labels),
		}

		fmt.Printf("[MIGRATE] Creating GitLab issue for GitHub issue #%d\n", issueID)
		newIssue, _, err := glClient.Issues.CreateIssue(req.Target.ProjectID, createOpts)
		if err != nil {
			fmt.Printf("[ERROR] Failed to create GitLab issue: %v\n", err)
			result.Failed = append(result.Failed, models.MigrationStatus{
				OriginalID: issueID,
				Error:      err.Error(),
			})
			continue
		}

		fmt.Printf("[SUCCESS] Created GitLab issue #%d for GitHub issue #%d\n", newIssue.IID, issueID)

		// Process comments
		comments, _, err := ghClient.Issues.ListComments(ctx, req.Source.Owner, req.Source.Repo, issueID, nil)
		if err == nil {
			fmt.Printf("[MIGRATE] Processing %d comments for issue #%d\n", len(comments), issueID)
			for i, comment := range comments {
				fmt.Printf("[MIGRATE] Processing comment %d/%d\n", i+1, len(comments))
				processedComment := processImagesWithLogging(
					comment.GetBody(),
					req.Target.ProjectID,
					req.Target.Token,
					req.Target.BaseURL,
					req.Source.Token, // Pass GitHub token for authenticated download
				)
				// Include comment timestamp
				commentHeader := fmt.Sprintf("**@%s** commented on %s",
					comment.User.GetLogin(),
					comment.GetCreatedAt().Format("2006-01-02 15:04:05 UTC"))
				if comment.UpdatedAt != nil && comment.GetUpdatedAt().After(comment.GetCreatedAt().Time) {
					commentHeader += fmt.Sprintf(" _(edited %s)_", comment.GetUpdatedAt().Format("2006-01-02 15:04:05 UTC"))
				}
				body := fmt.Sprintf("%s\n\n%s", commentHeader, processedComment)
				noteOpts := &gitlab.CreateIssueNoteOptions{
					Body: &body,
				}
				_, _, err := glClient.Notes.CreateIssueNote(req.Target.ProjectID, newIssue.IID, noteOpts)
				if err != nil {
					fmt.Printf("[WARNING] Failed to create comment: %v\n", err)
				}
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

func migrateGLtoGHFinal(req models.MigrationRequest, ginCtx *gin.Context) models.MigrationResult {
	result := models.MigrationResult{
		Success: []models.MigrationStatus{},
		Failed:  []models.MigrationStatus{},
	}

	glClient, _ := gitlab.NewClient(req.Source.Token, gitlab.WithBaseURL(req.Source.BaseURL))
	ghClient := github.NewClient(nil).WithAuthToken(req.Target.Token)
	ctx := context.Background()

	for _, issueID := range req.IssueIDs {
		fmt.Printf("[MIGRATE] Processing GitLab issue #%d\n", issueID)

		issue, _, err := glClient.Issues.GetIssue(req.Source.ProjectID, issueID)
		if err != nil {
			fmt.Printf("[ERROR] Failed to fetch issue #%d: %v\n", issueID, err)
			result.Failed = append(result.Failed, models.MigrationStatus{
				OriginalID: issueID,
				Error:      err.Error(),
			})
			continue
		}

		// Fix relative URLs
		processedBody := fixGitLabURLsFinal(issue.Description, req.Source.BaseURL)

		// Create migration header with detailed timestamp information
		migrationHeader := fmt.Sprintf("### 🔄 Migrated from GitLab\n\n")
		migrationHeader += fmt.Sprintf("**Original Issue:** %s\n", issue.WebURL)
		migrationHeader += fmt.Sprintf("**Original Author:** @%s\n", issue.Author.Username)
		migrationHeader += fmt.Sprintf("**Created:** %s\n", issue.CreatedAt.Format("2006-01-02 15:04:05 UTC"))
		migrationHeader += fmt.Sprintf("**Last Updated:** %s\n", issue.UpdatedAt.Format("2006-01-02 15:04:05 UTC"))
		if issue.State == "closed" && issue.ClosedAt != nil {
			migrationHeader += fmt.Sprintf("**Closed:** %s\n", issue.ClosedAt.Format("2006-01-02 15:04:05 UTC"))
		}
		migrationHeader += fmt.Sprintf("**State:** %s\n\n", issue.State)
		migrationHeader += "---\n\n"

		body := migrationHeader + processedBody

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

		fmt.Printf("[MIGRATE] Creating GitHub issue for GitLab issue #%d\n", issueID)
		newIssue, _, err := ghClient.Issues.Create(ctx, req.Target.Owner, req.Target.Repo, createReq)
		if err != nil {
			fmt.Printf("[ERROR] Failed to create GitHub issue: %v\n", err)
			result.Failed = append(result.Failed, models.MigrationStatus{
				OriginalID: issueID,
				Error:      err.Error(),
			})
			continue
		}

		fmt.Printf("[SUCCESS] Created GitHub issue #%d for GitLab issue #%d\n", newIssue.GetNumber(), issueID)

		// Process notes
		notes, _, err := glClient.Notes.ListIssueNotes(req.Source.ProjectID, issueID, nil)
		if err == nil {
			fmt.Printf("[MIGRATE] Processing %d notes for issue #%d\n", len(notes), issueID)
			for _, note := range notes {
				processedNote := fixGitLabURLsFinal(note.Body, req.Source.BaseURL)
				// Include note timestamp
				commentHeader := fmt.Sprintf("**@%s** commented on %s",
					note.Author.Username,
					note.CreatedAt.Format("2006-01-02 15:04:05 UTC"))
				if note.UpdatedAt != nil && note.UpdatedAt.After(*note.CreatedAt) {
					commentHeader += fmt.Sprintf(" _(edited %s)_", note.UpdatedAt.Format("2006-01-02 15:04:05 UTC"))
				}
				body := fmt.Sprintf("%s\n\n%s", commentHeader, processedNote)
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

// processImagesWithLogging finds, downloads, and re-uploads images with logging
func processImagesWithLogging(content string, projectID int, token string, baseURL string, sourceToken string) string {
	if content == "" {
		return content
	}

	fmt.Println("[IMAGE] Scanning content for images...")

	// Find all image URLs
	imageURLs := findImageURLs(content)
	fmt.Printf("[IMAGE] Found %d image(s) to process\n", len(imageURLs))

	if len(imageURLs) == 0 {
		return content
	}

	urlMap := make(map[string]string)
	client := &http.Client{}

	for i, imgURL := range imageURLs {
		fmt.Printf("[IMAGE] Processing image %d/%d: %s\n", i+1, len(imageURLs), imgURL)

		// Download image with authentication
		fmt.Printf("[IMAGE] Downloading image from %s\n", imgURL)
		
		// Create request with authentication header
		req, err := http.NewRequest("GET", imgURL, nil)
		if err != nil {
			fmt.Printf("[ERROR] Failed to create request: %v\n", err)
			continue
		}
		
		// Add GitHub authentication if it's a GitHub URL
		if strings.Contains(imgURL, "github.com") && sourceToken != "" {
			req.Header.Set("Authorization", "Bearer "+sourceToken)
			fmt.Printf("[IMAGE] Using GitHub authentication for download\n")
		}
		
		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("[ERROR] Failed to download image: %v\n", err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			fmt.Printf("[ERROR] Download failed with status %d\n", resp.StatusCode)
			resp.Body.Close()
			continue
		}

		imageData, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			fmt.Printf("[ERROR] Failed to read image data: %v\n", err)
			continue
		}

		fmt.Printf("[IMAGE] Downloaded %d bytes\n", len(imageData))

		// Get filename
		filename := getImageFilename(imgURL)
		fmt.Printf("[IMAGE] Uploading as '%s' to GitLab project %d\n", filename, projectID)

		// Upload to GitLab
		newURL, err := uploadImageToGitLab(projectID, imageData, filename, token, baseURL)
		if err != nil {
			fmt.Printf("[ERROR] Failed to upload image: %v\n", err)
			continue
		}

		fmt.Printf("[SUCCESS] Image uploaded successfully. New URL: %s\n", newURL)
		urlMap[imgURL] = newURL
	}

	// Replace all old URLs with new ones and convert HTML to Markdown
	fmt.Printf("[IMAGE] Replacing %d image URL(s) in content\n", len(urlMap))
	result := content
	
	for oldURL, newURL := range urlMap {
		// Convert GitLab URL to relative format
		// Handle both old and new formats:
		// Old: https://gitlab.com/uploads/xxx/filename.png -> /uploads/xxx/filename.png
		// New: https://gitlab.com/-/project/74006604/uploads/xxx/filename.png -> /-/project/74006604/uploads/xxx/filename.png
		relativeURL := newURL

		// Check for new format first
		if strings.Contains(newURL, "/-/project/") {
			idx := strings.Index(newURL, "/-/project/")
			if idx != -1 {
				relativeURL = newURL[idx:]
			}
		} else if strings.Contains(newURL, "/uploads/") {
			// Old format
			parts := strings.Split(newURL, "/uploads/")
			if len(parts) > 1 {
				relativeURL = "/uploads/" + parts[1]
			}
		}
		
		// Find and replace HTML img tags with Markdown format
		imgTagRegex := regexp.MustCompile(`<img[^>]*\ssrc=["']` + regexp.QuoteMeta(oldURL) + `["'][^>]*>`)
		matches := imgTagRegex.FindAllString(result, -1)
		for _, match := range matches {
			// Extract alt text if present
			altRegex := regexp.MustCompile(`alt=["']([^"']*)["']`)
			altMatch := altRegex.FindStringSubmatch(match)
			altText := "Image"
			if len(altMatch) > 1 && altMatch[1] != "" {
				altText = altMatch[1]
			}
			
			// Replace HTML with Markdown
			markdownImg := fmt.Sprintf("![%s](%s)", altText, relativeURL)
			result = strings.ReplaceAll(result, match, markdownImg)
		}
		
		// Also replace if it's already in Markdown format
		result = strings.ReplaceAll(result, `(`+oldURL+`)`, `(`+relativeURL+`)`)
	}

	return result
}

// findImageURLs extracts all image URLs from content
func findImageURLs(content string) []string {
	var urls []string
	seen := make(map[string]bool)

	// Pattern 1: HTML img tags
	imgTagRegex := regexp.MustCompile(`<img[^>]*\ssrc=["']([^"']+)["'][^>]*>`)
	matches := imgTagRegex.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) > 1 {
			url := match[1]
			if !seen[url] && isValidImageURL(url) {
				urls = append(urls, url)
				seen[url] = true
				fmt.Printf("[IMAGE] Found HTML img: %s\n", url)
			}
		}
	}

	// Pattern 2: Markdown images
	mdImageRegex := regexp.MustCompile(`!\[([^\]]*)\]\(([^)]+)\)`)
	matches = mdImageRegex.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) > 2 {
			url := match[2]
			if !seen[url] && isValidImageURL(url) {
				urls = append(urls, url)
				seen[url] = true
				fmt.Printf("[IMAGE] Found Markdown img: %s\n", url)
			}
		}
	}

	return urls
}

// isValidImageURL checks if URL is an image
func isValidImageURL(url string) bool {
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return false
	}

	lowerURL := strings.ToLower(url)

	// Check for file extensions
	imageExts := []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".bmp"}
	for _, ext := range imageExts {
		if strings.Contains(lowerURL, ext) {
			return true
		}
	}

	// Check for known image hosting patterns
	imagePatterns := []string{
		"githubusercontent.com",
		"github.com/user-attachments/assets",
		"gitlab.com/uploads",
		"imgur.com",
	}

	for _, pattern := range imagePatterns {
		if strings.Contains(url, pattern) {
			return true
		}
	}

	return false
}

// uploadImageToGitLab uploads image to GitLab
func uploadImageToGitLab(projectID int, imageData []byte, filename string, token string, baseURL string) (string, error) {
	url := fmt.Sprintf("%s/api/v4/projects/%d/uploads", baseURL, projectID)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return "", err
	}

	if _, err := part.Write(imageData); err != nil {
		return "", err
	}

	if err := writer.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return "", err
	}

	req.Header.Set("PRIVATE-TOKEN", token)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusForbidden {
			return "", fmt.Errorf("upload failed with status 403 Forbidden - Check that your GitLab token has 'api' scope and write access to project %d: %s", projectID, string(respBody))
		}
		return "", fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var uploadResult struct {
		URL string `json:"url"`
	}

	if err := json.Unmarshal(respBody, &uploadResult); err != nil {
		return "", err
	}

	if strings.HasPrefix(uploadResult.URL, "/") {
		return baseURL + uploadResult.URL, nil
	}
	return uploadResult.URL, nil
}

// getImageFilename extracts filename from URL
func getImageFilename(url string) string {
	// For GitHub attachments
	if strings.Contains(url, "github.com/user-attachments/assets") {
		return "github-attachment.png"
	}

	// Remove query parameters
	cleanURL := url
	if idx := strings.Index(url, "?"); idx != -1 {
		cleanURL = url[:idx]
	}

	parts := strings.Split(cleanURL, "/")
	if len(parts) > 0 {
		filename := parts[len(parts)-1]
		if filename != "" && strings.Contains(filename, ".") {
			return filename
		}
	}

	return "image.png"
}

// fixGitLabURLsFinal converts relative GitLab URLs to absolute
func fixGitLabURLsFinal(content string, baseURL string) string {
	if content == "" {
		return content
	}

	// Handle new GitLab format: /-/project/{id}/uploads/...
	content = strings.ReplaceAll(content, `](/-/project/`, `](`+baseURL+`/-/project/`)
	content = strings.ReplaceAll(content, `="/-/project/`, `="`+baseURL+`/-/project/`)

	// Also handle old format for backwards compatibility
	content = strings.ReplaceAll(content, `](/uploads/`, `](`+baseURL+`/uploads/`)
	content = strings.ReplaceAll(content, `="/uploads/`, `="`+baseURL+`/uploads/`)

	return content
}
