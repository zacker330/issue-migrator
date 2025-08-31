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
		fmt.Printf("[MIGRATE] Processing attachments in issue #%d body\n", issueID)
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

		description := fmt.Sprintf("*Migrated from GitHub: %s*\n\n%s", issue.GetHTMLURL(), processedBody)
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
				body := fmt.Sprintf("**@%s commented:**\n\n%s", comment.User.GetLogin(), processedComment)
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
		processedBody := processGitLabToGitHub(issue.Description, req.Source.BaseURL, req.Source.Token, req.Target.Token, req.Target.Session)
		
		// Create migration header with better formatting
		migrationHeader := fmt.Sprintf("### 🔄 Migrated from GitLab\n\n")
		migrationHeader += fmt.Sprintf("**Original Issue:** %s\n", issue.WebURL)
		migrationHeader += fmt.Sprintf("**Original Author:** @%s\n", issue.Author.Username)
		migrationHeader += fmt.Sprintf("**Created:** %s\n\n", issue.CreatedAt.Format("2006-01-02"))
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
				processedNote := processGitLabToGitHub(note.Body, req.Source.BaseURL, req.Source.Token, req.Target.Token, req.Target.Session)
				body := fmt.Sprintf("**@%s commented:**\n\n%s", note.Author.Username, processedNote)
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
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, string(respBody))
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
func processGitLabToGitHub(content string, gitlabURL string, gitlabToken string, githubToken string, githubSession string) string {
	if content == "" {
		return content
	}

	// First ensure all URLs are absolute
	content = fixGitLabAttachmentURLs(content, gitlabURL)
	
	// Find all GitLab attachment URLs
	attachments := findGitLabAttachments(content, gitlabURL)
	
	if len(attachments) == 0 {
		return content
	}
	
	fmt.Printf("[ATTACH] Found %d GitLab attachments to process\n", len(attachments))
	
	urlMap := make(map[string]string)
	client := &http.Client{}
	
	for _, attachment := range attachments {
		fmt.Printf("[ATTACH] Processing GitLab attachment: %s\n", attachment.URL)
		
		// Download from GitLab
		req, err := http.NewRequest("GET", attachment.URL, nil)
		if err != nil {
			fmt.Printf("[ERROR] Failed to create request: %v\n", err)
			continue
		}
		
		// Add GitLab token if it's a private project
		if gitlabToken != "" {
			req.Header.Set("PRIVATE-TOKEN", gitlabToken)
		}
		
		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("[ERROR] Failed to download from GitLab: %v\n", err)
			continue
		}
		
		if resp.StatusCode != http.StatusOK {
			fmt.Printf("[WARNING] GitLab download returned status %d\n", resp.StatusCode)
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
		
		// Try to upload to GitHub
		// Note: This is experimental and may not work without proper session auth
		githubURL, err := UploadToGitHub(data, attachment.Filename, githubToken, githubSession)
		if err != nil {
			fmt.Printf("[WARNING] GitHub upload failed: %v\n", err)
			fmt.Printf("[INFO] File will remain on GitLab: %s\n", attachment.URL)
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
	
	// Pattern for GitLab uploads
	uploadPattern := regexp.MustCompile(gitlabURL + `/uploads/([a-f0-9]+)/([^"'\s\)]+)`)
	matches := uploadPattern.FindAllStringSubmatch(content, -1)
	
	for _, match := range matches {
		if len(match) > 2 {
			fullURL := match[0]
			filename := match[2]
			
			if !seen[fullURL] {
				attachments = append(attachments, GitLabAttachment{
					URL:      fullURL,
					Filename: filename,
				})
				seen[fullURL] = true
			}
		}
	}
	
	return attachments
}

// fixGitLabAttachmentURLs converts relative GitLab URLs to absolute and ensures proper formatting
func fixGitLabAttachmentURLs(content string, baseURL string) string {
	if content == "" {
		return content
	}

	// First, convert relative URLs to absolute
	content = strings.ReplaceAll(content, `](/uploads/`, `](`+baseURL+`/uploads/`)
	content = strings.ReplaceAll(content, `="/uploads/`, `="`+baseURL+`/uploads/`)
	content = strings.ReplaceAll(content, `='/uploads/`, `='`+baseURL+`/uploads/`)
	
	// Fix any GitLab URLs that are missing the base URL
	// Pattern: ![text](uploads/...) -> ![text](https://gitlab.com/uploads/...)
	uploadsRegex := regexp.MustCompile(`\]\((uploads/[^)]+)\)`)
	content = uploadsRegex.ReplaceAllString(content, `](`+baseURL+`/$1)`)
	
	// Ensure images with just filename get proper URL
	// Pattern: ![Image](xxx) where xxx doesn't start with http
	imageRegex := regexp.MustCompile(`!\[([^\]]*)\]\(([^)]+)\)`)
	matches := imageRegex.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) > 2 {
			originalURL := match[2]
			// If URL doesn't start with http and contains /uploads/
			if !strings.HasPrefix(originalURL, "http") && strings.Contains(originalURL, "uploads/") {
				fullURL := baseURL + "/" + strings.TrimPrefix(originalURL, "/")
				oldPattern := fmt.Sprintf("![%s](%s)", match[1], originalURL)
				newPattern := fmt.Sprintf("![%s](%s)", match[1], fullURL)
				content = strings.ReplaceAll(content, oldPattern, newPattern)
			}
		}
	}
	
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