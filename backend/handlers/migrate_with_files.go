package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/go-github/v57/github"
	"github.com/issue-migrator/backend/models"
	"github.com/xanzy/go-gitlab"
)

// MigrateWithFiles handles migration with file and image transfer
func MigrateWithFiles(c *gin.Context) {
	var req models.MigrationRequest
	log.Println("Starting migration with file support")

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	fmt.Printf("[MIGRATE] Starting migration: %s\n", req.Direction)
	fmt.Printf("[MIGRATE] Source: %+v\n", req.Source)
	fmt.Printf("[MIGRATE] Target: %+v\n", req.Target)
	fmt.Printf("[MIGRATE] Issues to migrate: %v\n", req.IssueIDs)

	results := models.MigrationResult{
		Success: []models.MigrationStatus{},
		Failed:  []models.MigrationStatus{},
	}

	switch req.Direction {
	case "github-to-gitlab":
		results = migrateGHtoGLWithFiles(req, c)
	case "gitlab-to-github":
		results = migrateGLtoGHWithFiles(req, c)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid migration direction"})
		return
	}

	fmt.Printf("[MIGRATE] Migration completed. Success: %d, Failed: %d\n",
		len(results.Success), len(results.Failed))

	c.JSON(http.StatusOK, results)
}

func migrateGHtoGLWithFiles(req models.MigrationRequest, ginCtx *gin.Context) models.MigrationResult {
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

		// Process all attachments (images and files) in issue body
		fmt.Printf("[MIGRATE] Processing attachments in issue #%d body1111\n", issueID)
		processedBody := processAttachments(
			issue.GetBody(),
			req.Target.ProjectID,
			req.Target.Token,
			req.Target.BaseURL,
			req.Source.Token,
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
				processedComment := processAttachments(
					comment.GetBody(),
					req.Target.ProjectID,
					req.Target.Token,
					req.Target.BaseURL,
					req.Source.Token,
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

func migrateGLtoGHWithFiles(req models.MigrationRequest, ginCtx *gin.Context) models.MigrationResult {
	result := models.MigrationResult{
		Success: []models.MigrationStatus{},
		Failed:  []models.MigrationStatus{},
	}

	glClient, _ := gitlab.NewClient(req.Source.Token, gitlab.WithBaseURL(req.Source.BaseURL))
	ghClient := github.NewClient(nil).WithAuthToken(req.Target.Token)
	ctx := context.Background()

	fmt.Println("[INFO] GitLab to GitHub migration: Attempting to upload files to GitHub")
	fmt.Println("[INFO] Note: GitHub upload API is unofficial and may require browser session")

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

		// Try to download and re-upload GitLab attachments to GitHub
		processedBody := processGitLabToGitHub(issue.Description, req.Source.BaseURL, req.Source.ProjectID, req.Source.Token, req.Target.Token, req.Target.Session, req.Source.Session, req.Target.Owner, req.Target.Repo)

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

		// Process notes with timestamps
		notes, _, err := glClient.Notes.ListIssueNotes(req.Source.ProjectID, issueID, nil)
		if err == nil {
			fmt.Printf("[MIGRATE] Processing %d notes for issue #%d\n", len(notes), issueID)
			for _, note := range notes {
				processedNote := processGitLabToGitHub(note.Body, req.Source.BaseURL, req.Source.ProjectID, req.Source.Token, req.Target.Token, req.Target.Session, req.Source.Session, req.Target.Owner, req.Target.Repo)
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

// processAttachments handles both images and files
func processAttachments(content string, projectID int, token string, baseURL string, sourceToken string) string {
	if content == "" {
		return content
	}

	fmt.Println("[ATTACH] Scanning content for attachments...")

	// Find all attachment URLs (images and files)
	attachmentURLs := findAllAttachments(content)
	fmt.Printf("[ATTACH] Found %d attachment(s) to process\n", len(attachmentURLs))

	if len(attachmentURLs) == 0 {
		return content
	}

	urlMap := make(map[string]AttachmentInfo)
	client := &http.Client{}

	for i, attachment := range attachmentURLs {
		fmt.Printf("[ATTACH] Processing attachment %d/%d: %s\n", i+1, len(attachmentURLs), attachment.URL)

		// Download attachment with authentication
		req, err := http.NewRequest("GET", attachment.URL, nil)
		if err != nil {
			fmt.Printf("[ERROR] Failed to create request: %v\n", err)
			continue
		}

		// Add GitHub authentication if needed
		if strings.Contains(attachment.URL, "github.com") && sourceToken != "" {
			req.Header.Set("Authorization", "Bearer "+sourceToken)
			fmt.Printf("[ATTACH] Using GitHub authentication for download\n")
		}

		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("[ERROR] Failed to download attachment: %v\n", err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			fmt.Printf("[ERROR] Download failed with status %d\n", resp.StatusCode)
			resp.Body.Close()
			continue
		}

		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			fmt.Printf("[ERROR] Failed to read attachment data: %v\n", err)
			continue
		}

		fmt.Printf("[ATTACH] Downloaded %d bytes\n", len(data))

		// Upload to GitLab
		filename := getFilename(attachment.URL, attachment.OriginalText)
		fmt.Printf("[ATTACH] Uploading as '%s' to GitLab project %d\n", filename, projectID)

		newURL, err := uploadFileToGitLab(projectID, data, filename, token, baseURL)
		if err != nil {
			fmt.Printf("[ERROR] Failed to upload file: %v\n", err)
			continue
		}

		fmt.Printf("[SUCCESS] File uploaded successfully. New URL: %s\n", newURL)
		attachment.NewURL = newURL
		urlMap[attachment.URL] = attachment
	}

	// Replace all old URLs with new ones
	fmt.Printf("[ATTACH] Replacing %d attachment URL(s) in content\n", len(urlMap))
	result := content

	for oldURL, attachment := range urlMap {
		// Convert to relative URL for GitLab
		relativeURL := attachment.NewURL
		if strings.Contains(attachment.NewURL, "/uploads/") {
			parts := strings.Split(attachment.NewURL, "/uploads/")
			if len(parts) > 1 {
				relativeURL = "/uploads/" + parts[1]
			}
		}

		// Handle different types of content
		if attachment.IsImage {
			// Convert HTML img tags to Markdown
			if strings.Contains(attachment.OriginalText, "<img") {
				imgTagRegex := regexp.MustCompile(`<img[^>]*\ssrc=["']` + regexp.QuoteMeta(oldURL) + `["'][^>]*>`)
				matches := imgTagRegex.FindAllString(result, -1)
				for _, match := range matches {
					// Extract alt text
					altRegex := regexp.MustCompile(`alt=["']([^"']*)["']`)
					altMatch := altRegex.FindStringSubmatch(match)
					altText := "Image"
					if len(altMatch) > 1 && altMatch[1] != "" {
						altText = altMatch[1]
					}
					// Replace with Markdown
					markdownImg := fmt.Sprintf("![%s](%s)", altText, relativeURL)
					result = strings.ReplaceAll(result, match, markdownImg)
				}
			} else {
				// Already in Markdown format
				result = strings.ReplaceAll(result, `(`+oldURL+`)`, `(`+relativeURL+`)`)
			}
		} else {
			// For non-image files, create a link
			if strings.Contains(attachment.OriginalText, "<a") {
				// Replace HTML link
				linkRegex := regexp.MustCompile(`<a[^>]*\shref=["']` + regexp.QuoteMeta(oldURL) + `["'][^>]*>([^<]*)</a>`)
				matches := linkRegex.FindAllStringSubmatch(result, -1)
				for _, match := range matches {
					linkText := "Download"
					if len(match) > 1 && match[1] != "" {
						linkText = match[1]
					}
					markdownLink := fmt.Sprintf("[%s](%s)", linkText, relativeURL)
					result = strings.ReplaceAll(result, match[0], markdownLink)
				}
			} else {
				// Direct URL replacement
				result = strings.ReplaceAll(result, oldURL, relativeURL)
			}
		}
	}

	return result
}

// AttachmentInfo holds information about an attachment
type AttachmentInfo struct {
	URL          string
	NewURL       string
	IsImage      bool
	OriginalText string
}

// findAllAttachments finds all attachment URLs (images and files)
func findAllAttachments(content string) []AttachmentInfo {
	var attachments []AttachmentInfo
	seen := make(map[string]bool)

	// Pattern 1: HTML img tags
	imgTagRegex := regexp.MustCompile(`<img[^>]*\ssrc=["']([^"']+)["'][^>]*>`)
	matches := imgTagRegex.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) > 1 && !seen[match[1]] && isValidURL(match[1]) {
			attachments = append(attachments, AttachmentInfo{
				URL:          match[1],
				IsImage:      true,
				OriginalText: match[0],
			})
			seen[match[1]] = true
			fmt.Printf("[ATTACH] Found HTML img: %s\n", match[1])
		}
	}

	// Pattern 2: Markdown images
	mdImageRegex := regexp.MustCompile(`!\[([^\]]*)\]\(([^)]+)\)`)
	matches = mdImageRegex.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) > 2 && !seen[match[2]] && isValidURL(match[2]) {
			attachments = append(attachments, AttachmentInfo{
				URL:          match[2],
				IsImage:      true,
				OriginalText: match[0],
			})
			seen[match[2]] = true
			fmt.Printf("[ATTACH] Found Markdown img: %s\n", match[2])
		}
	}

	// Pattern 3: HTML links to files
	linkRegex := regexp.MustCompile(`<a[^>]*\shref=["']([^"']+)["'][^>]*>([^<]*)</a>`)
	matches = linkRegex.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) > 1 && !seen[match[1]] && isValidURL(match[1]) && isFileURL(match[1]) {
			attachments = append(attachments, AttachmentInfo{
				URL:          match[1],
				IsImage:      false,
				OriginalText: match[0],
			})
			seen[match[1]] = true
			fmt.Printf("[ATTACH] Found file link: %s\n", match[1])
		}
	}

	// Pattern 4: Markdown links to files
	mdLinkRegex := regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	matches = mdLinkRegex.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) > 2 && !seen[match[2]] && !strings.HasPrefix(match[0], "!") &&
			isValidURL(match[2]) && isFileURL(match[2]) {
			attachments = append(attachments, AttachmentInfo{
				URL:          match[2],
				IsImage:      false,
				OriginalText: match[0],
			})
			seen[match[2]] = true
			fmt.Printf("[ATTACH] Found Markdown file link: %s\n", match[2])
		}
	}

	return attachments
}

// isValidURL checks if URL is valid
func isValidURL(url string) bool {
	return strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")
}

// isFileURL checks if URL points to a file (including images)
func isFileURL(url string) bool {
	lowerURL := strings.ToLower(url)

	// Check for file extensions
	fileExts := []string{
		// Images
		".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".bmp",
		// Documents
		".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx",
		".txt", ".rtf", ".odt", ".ods", ".odp",
		// Archives
		".zip", ".tar", ".gz", ".rar", ".7z",
		// Code
		".js", ".ts", ".py", ".go", ".java", ".c", ".cpp", ".h",
		".json", ".xml", ".yaml", ".yml", ".toml", ".ini", ".conf",
		// Other
		".csv", ".log", ".sql", ".sh", ".bat", ".exe", ".dmg", ".deb", ".rpm",
	}

	for _, ext := range fileExts {
		if strings.Contains(lowerURL, ext) {
			return true
		}
	}

	// Check for known file hosting patterns
	filePatterns := []string{
		"github.com/user-attachments/assets",
		"gitlab.com/uploads",
		"githubusercontent.com",
	}

	for _, pattern := range filePatterns {
		if strings.Contains(url, pattern) {
			return true
		}
	}

	return false
}

// uploadFileToGitLab uploads any file to GitLab
func uploadFileToGitLab(projectID int, data []byte, filename string, token string, baseURL string) (string, error) {
	url := fmt.Sprintf("%s/api/v4/projects/%d/uploads", baseURL, projectID)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return "", err
	}

	if _, err := part.Write(data); err != nil {
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

// getFilename extracts or generates filename
func getFilename(url string, originalText string) string {
	// Try to get filename from URL
	cleanURL := url
	if idx := strings.Index(url, "?"); idx != -1 {
		cleanURL = url[:idx]
	}

	// For GitHub attachments, try to extract original filename
	if strings.Contains(url, "github.com/user-attachments/assets") {
		// Try to extract from alt text or link text
		if strings.Contains(originalText, "alt=") {
			altRegex := regexp.MustCompile(`alt=["']([^"']*)["']`)
			if match := altRegex.FindStringSubmatch(originalText); len(match) > 1 {
				name := match[1]
				// Add extension if missing
				if !strings.Contains(name, ".") {
					name += guessExtension(url)
				}
				return sanitizeFilename(name)
			}
		}
		// Generate a name
		return "attachment" + guessExtension(url)
	}

	// Get the last part of the path
	filename := path.Base(cleanURL)
	if filename != "" && filename != "." && filename != "/" {
		return sanitizeFilename(filename)
	}

	return "attachment"
}

// sanitizeFilename removes unsafe characters
func sanitizeFilename(name string) string {
	// Remove or replace unsafe characters
	name = strings.ReplaceAll(name, " ", "_")
	name = regexp.MustCompile(`[^\w\-\.]`).ReplaceAllString(name, "")
	if name == "" {
		return "file"
	}
	return name
}

// guessExtension tries to guess file extension
func guessExtension(url string) string {
	lowerURL := strings.ToLower(url)

	extensions := map[string]string{
		"png":  ".png",
		"jpg":  ".jpg",
		"jpeg": ".jpeg",
		"gif":  ".gif",
		"pdf":  ".pdf",
		"zip":  ".zip",
		"doc":  ".doc",
		"docx": ".docx",
	}

	for key, ext := range extensions {
		if strings.Contains(lowerURL, key) {
			return ext
		}
	}

	return ""
}

// processGitLabToGitHub attempts to download GitLab files and upload to GitHub
func processGitLabToGitHub(content string, gitlabURL string, projectID int, gitlabToken string, githubToken string, githubSession string, gitlabSession string, githubOwner string, githubRepo string) string {
	if content == "" {
		return content
	}

	// First ensure all URLs are absolute and fix any old format URLs
	content = fixGitLabAttachmentURLs(content, gitlabURL, projectID)

	// Find all GitLab attachment URLs
	attachments := findGitLabAttachments(content, gitlabURL)

	if len(attachments) == 0 {
		return content
	}

	fmt.Printf("[ATTACH] Found %d GitLab attachments to process\n", len(attachments))

	urlMap := make(map[string]string)
	client := &http.Client{}

	for _, attachment := range attachments {
		fmt.Printf("[ATTACH] Processing GitLab attachment11111: %s\n", attachment.URL)

		// Download from GitLab
		req, err := http.NewRequest("GET", attachment.URL, nil)
		if err != nil {
			fmt.Printf("[ERROR] Failed to create request: %v\n", err)
			continue
		}

		// Try using GitLab session cookie first (for /uploads/ endpoints)
		if gitlabSession != "" {
			req.Header.Set("Cookie", fmt.Sprintf("_gitlab_session=%s", gitlabSession))
			fmt.Printf("[AUTH] Using GitLab session cookie for download\n")
		} else if gitlabToken != "" {
			// Fallback to API token (might work for some endpoints)
			req.Header.Set("PRIVATE-TOKEN", gitlabToken)
			fmt.Printf("[AUTH] Using GitLab API token for download\n")
		}

		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("[ERROR] Failed to download from GitLab: %v\n", err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusUnauthorized {
				fmt.Printf("[WARNING] Cannot download GitLab attachment (status %d): %s\n", resp.StatusCode, attachment.URL)
				if gitlabSession == "" {
					fmt.Printf("[INFO] GitLab /uploads/ URLs require browser session authentication.\n")
					fmt.Printf("[INFO] To download attachments from private repos, provide a GitLab session cookie.\n")
					fmt.Printf("[INFO] How to get GitLab session cookie:\n")
					fmt.Printf("[INFO]   1. Log in to GitLab in your browser\n")
					fmt.Printf("[INFO]   2. Open Developer Tools (F12)\n")
					fmt.Printf("[INFO]   3. Go to Application/Storage -> Cookies\n")
					fmt.Printf("[INFO]   4. Find and copy the '_gitlab_session' cookie value\n")
				} else {
					fmt.Printf("[INFO] Session cookie provided but still cannot access. The session may be expired or invalid.\n")
				}
				fmt.Printf("[INFO] Alternative workarounds:\n")
				fmt.Printf("[INFO]   1. Make the GitLab project public temporarily during migration\n")
				fmt.Printf("[INFO]   2. Manually download and re-upload attachments after migration\n")
				// Keep the original URL in the content
				fmt.Printf("[INFO] Keeping original GitLab URL: %s\n", attachment.URL)
			} else {
				fmt.Printf("[WARNING] GitLab download returned status %d for URL: %s\n", resp.StatusCode, attachment.URL)
				fmt.Printf("[DEBUG] Response: %s\n", string(bodyBytes))
			}
			resp.Body.Close()
			continue
		}

		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			fmt.Printf("[ERROR] Failed to read data: %v\n", err)
			continue
		}

		fmt.Printf("[ATTACH] Downloaded %d bytes from GitLab\n", len(data))

		// Detect file type and add extension if missing
		filename := attachment.Filename
		if !strings.Contains(filename, ".") || strings.HasSuffix(filename, "/Image") || strings.HasSuffix(filename, "/image") {
			// No extension or generic "Image" name - detect from content
			detectedExt := detectFileExtension(data)
			if detectedExt != "" {
				// If filename is just "Image", replace it with a better name
				if filename == "Image" || filename == "image" || strings.HasSuffix(filename, "/Image") || strings.HasSuffix(filename, "/image") {
					filename = "image" + detectedExt
				} else if !strings.HasSuffix(filename, detectedExt) {
					filename = filename + detectedExt
				}
				fmt.Printf("[ATTACH] Detected file type: %s, using filename: %s\n", detectedExt, filename)
			}
		}

		// Try to upload to GitHub (experimental - often fails due to GitHub's security)
		// Skip if no session provided since it won't work anyway
		if githubSession == "" {
			fmt.Printf("[INFO] Skipping GitHub upload (no session cookie provided)\n")
			fmt.Printf("[INFO] File will remain on GitLab: %s\n", attachment.URL)
			continue
		}

		// Try to get repository ID if we have owner and repo
		var repoID string
		if githubOwner != "" && githubRepo != "" && githubToken != "" {
			repoID = getGitHubRepoID(githubOwner, githubRepo, githubToken)
			if repoID != "" {
				fmt.Printf("[INFO] Using repository ID: %s for uploads\n", repoID)
			}
		}

		githubURL, err := UploadToGitHubWithRepo(data, filename, githubToken, githubSession, repoID)
		if err != nil {
			fmt.Printf("[WARNING] GitHub upload failed: %v\n", err)

			// Only show detailed message once
			if !strings.Contains(content, "_GitHub_upload_notice_shown_") {
				fmt.Printf("[INFO] ========================================\n")
				fmt.Printf("[INFO] GitHub Upload Limitation:\n")
				fmt.Printf("[INFO] GitHub's file upload API requires a complete browser session\n")
				fmt.Printf("[INFO] including CSRF tokens and other security measures that cannot\n")
				fmt.Printf("[INFO] be easily obtained programmatically.\n")
				fmt.Printf("[INFO] \n")
				fmt.Printf("[INFO] Current behavior:\n")
				fmt.Printf("[INFO] - Files remain hosted on GitLab\n")
				fmt.Printf("[INFO] - Links are preserved in migrated issues\n")
				fmt.Printf("[INFO] - Images will display if GitLab repo is public\n")
				fmt.Printf("[INFO] \n")
				fmt.Printf("[INFO] Alternatives:\n")
				fmt.Printf("[INFO] 1. Make GitLab repo public during migration\n")
				fmt.Printf("[INFO] 2. Manually re-upload important files after migration\n")
				fmt.Printf("[INFO] 3. Use GitHub Actions for automated uploads\n")
				fmt.Printf("[INFO] ========================================\n")
				content = content + "<!-- _GitHub_upload_notice_shown_ -->"
			}

			fmt.Printf("[INFO] File will remain on GitLab: %s\n", attachment.URL)
			// Don't replace the URL, keep it pointing to GitLab
			continue
		}

		fmt.Printf("[SUCCESS] Uploaded to GitHub: %s\n", githubURL)
		urlMap[attachment.URL] = githubURL
	}

	// Replace successful uploads
	result := content
	for oldURL, newURL := range urlMap {
		result = strings.ReplaceAll(result, oldURL, newURL)
	}

	return result
}

// GitLabAttachment represents a GitLab attachment
type GitLabAttachment struct {
	URL      string
	Filename string
}

// findGitLabAttachments finds all GitLab attachment URLs
func findGitLabAttachments(content string, gitlabURL string) []GitLabAttachment {
	var attachments []GitLabAttachment
	seen := make(map[string]bool)

	// Pattern for GitLab uploads (new format only, since we convert old to new)
	// Format: https://gitlab.com/-/project/74006604/uploads/c3365884e9152d7384859c7ac5a16ff4/Image
	projectUploadPattern := regexp.MustCompile(gitlabURL + `/-/project/(\d+)/uploads/([a-f0-9]+)/([^"'\s\)]+)`)

	matches := projectUploadPattern.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) > 3 {
			fullURL := match[0]
			projectID := match[1]
			hash := match[2]
			filename := match[3]

			if !seen[fullURL] {
				attachments = append(attachments, GitLabAttachment{
					URL:      fullURL,
					Filename: filename,
				})
				seen[fullURL] = true
				fmt.Printf("[ATTACH] Found GitLab attachment: project=%s, hash=%s, file=%s, url=%s\n", projectID, hash, filename, fullURL)
			}
		}
	}

	return attachments
}

// getGitHubRepoID fetches the repository ID from GitHub API
func getGitHubRepoID(owner string, repo string, token string) string {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s", owner, repo)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Printf("[WARNING] Failed to create request for repo ID: %v\n", err)
		return ""
	}

	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("[WARNING] Failed to fetch repo ID: %v\n", err)
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("[WARNING] Failed to get repo info: status %d\n", resp.StatusCode)
		return ""
	}

	var repoInfo struct {
		ID int64 `json:"id"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&repoInfo); err != nil {
		fmt.Printf("[WARNING] Failed to parse repo info: %v\n", err)
		return ""
	}

	return fmt.Sprintf("%d", repoInfo.ID)
}

// fixGitLabAttachmentURLs converts relative GitLab URLs to absolute and ensures proper formatting
func fixGitLabAttachmentURLs(content string, baseURL string, projectID int) string {
	if content == "" {
		return content
	}

	// Convert old format URLs to new format FIRST
	// Old: /uploads/xxx/file or https://gitlab.com/uploads/xxx/file
	// New: https://gitlab.com/-/project/{projectID}/uploads/xxx/file

	// Fix relative old format URLs
	content = strings.ReplaceAll(content, `](/uploads/`, `](/-/project/`+fmt.Sprintf("%d", projectID)+`/uploads/`)
	content = strings.ReplaceAll(content, `="/uploads/`, `="/-/project/`+fmt.Sprintf("%d", projectID)+`/uploads/`)
	content = strings.ReplaceAll(content, `='/uploads/`, `='/-/project/`+fmt.Sprintf("%d", projectID)+`/uploads/`)

	// Fix absolute old format URLs
	oldFormatRegex := regexp.MustCompile(regexp.QuoteMeta(baseURL) + `/uploads/([a-f0-9]+)/([^"'\s\)]+)`)
	content = oldFormatRegex.ReplaceAllString(content, baseURL+`/-/project/`+fmt.Sprintf("%d", projectID)+`/uploads/$1/$2`)

	// Now handle new format: convert relative to absolute
	content = strings.ReplaceAll(content, `](/-/project/`, `](`+baseURL+`/-/project/`)
	content = strings.ReplaceAll(content, `="/-/project/`, `="`+baseURL+`/-/project/`)
	content = strings.ReplaceAll(content, `='/-/project/`, `='`+baseURL+`/-/project/`)


	// Add a note about external dependencies for GitHub
	if strings.Contains(content, baseURL+"/uploads/") {
		header := fmt.Sprintf("_Note: This issue contains attachments hosted on GitLab (%s). ", baseURL)
		header += "These files will remain accessible as long as the GitLab project exists._\n\n"

		// Only add the header if it's not already there
		if !strings.Contains(content, header) {
			content = header + content
		}
	}

	return content
}

// detectFileExtension detects the file type from the binary content using magic bytes
func detectFileExtension(data []byte) string {
	if len(data) < 4 {
		return ""
	}

	// Check for common image formats using magic bytes
	// JPEG
	if len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return ".jpg"
	}

	// PNG
	if len(data) >= 8 && bytes.Equal(data[0:8], []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}) {
		return ".png"
	}

	// GIF
	if len(data) >= 6 && (bytes.Equal(data[0:6], []byte("GIF87a")) || bytes.Equal(data[0:6], []byte("GIF89a"))) {
		return ".gif"
	}

	// WebP
	if len(data) >= 12 && bytes.Equal(data[0:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")) {
		return ".webp"
	}

	// BMP
	if len(data) >= 2 && data[0] == 0x42 && data[1] == 0x4D {
		return ".bmp"
	}

	// ICO
	if len(data) >= 4 && data[0] == 0x00 && data[1] == 0x00 && data[2] == 0x01 && data[3] == 0x00 {
		return ".ico"
	}

	// TIFF (little-endian)
	if len(data) >= 4 && data[0] == 0x49 && data[1] == 0x49 && data[2] == 0x2A && data[3] == 0x00 {
		return ".tiff"
	}

	// TIFF (big-endian)
	if len(data) >= 4 && data[0] == 0x4D && data[1] == 0x4D && data[2] == 0x00 && data[3] == 0x2A {
		return ".tiff"
	}

	// SVG (XML-based, check for common SVG patterns)
	if len(data) >= 50 {
		header := string(data[:min(len(data), 200)])
		if strings.Contains(strings.ToLower(header), "<svg") || strings.Contains(strings.ToLower(header), "<?xml") && strings.Contains(strings.ToLower(header), "svg") {
			return ".svg"
		}
	}

	// HEIC
	if len(data) >= 12 && (bytes.Equal(data[4:8], []byte("ftypheic")) || bytes.Equal(data[4:8], []byte("ftypheix")) || bytes.Equal(data[4:8], []byte("ftyphevc"))) {
		return ".heic"
	}

	// AVIF
	if len(data) >= 12 && bytes.Equal(data[4:8], []byte("ftypavif")) {
		return ".avif"
	}

	// PDF
	if len(data) >= 5 && bytes.Equal(data[0:5], []byte("%PDF-")) {
		return ".pdf"
	}

	// ZIP
	if len(data) >= 4 && data[0] == 0x50 && data[1] == 0x4B && (data[2] == 0x03 || data[2] == 0x05 || data[2] == 0x07) && (data[3] == 0x04 || data[3] == 0x06 || data[3] == 0x08) {
		return ".zip"
	}

	// Check for video formats
	// MP4
	if len(data) >= 12 && (bytes.Equal(data[4:8], []byte("ftyp")) || bytes.Equal(data[4:8], []byte("ftypmp4")) || bytes.Equal(data[4:8], []byte("ftypisom"))) {
		return ".mp4"
	}

	// AVI
	if len(data) >= 12 && bytes.Equal(data[0:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("AVI ")) {
		return ".avi"
	}

	// MOV
	if len(data) >= 8 && bytes.Equal(data[4:8], []byte("ftypqt")) {
		return ".mov"
	}

	// WebM
	if len(data) >= 4 && data[0] == 0x1A && data[1] == 0x45 && data[2] == 0xDF && data[3] == 0xA3 {
		return ".webm"
	}

	// Default to empty string if type cannot be detected
	return ""
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
