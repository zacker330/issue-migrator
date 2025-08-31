package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

// GitHubUploadPolicy represents the response from GitHub's upload policy endpoint
type GitHubUploadPolicy struct {
	UploadURL           string            `json:"upload_url"`
	UploadAuthenticityToken string        `json:"upload_authenticity_token"`
	FormData            map[string]string `json:"form_data"`
	Asset               struct {
		ID          int    `json:"id"`
		Name        string `json:"name"`
		Size        int    `json:"size"`
		ContentType string `json:"content_type"`
		Href        string `json:"href"`
		OriginalName string `json:"original_name"`
	} `json:"asset"`
}

// GitHubUploadResponse represents the response after uploading
type GitHubUploadResponse struct {
	ID          string `json:"id"`
	URL         string `json:"url"`
	Href        string `json:"href"`
	OriginalName string `json:"original_name"`
}

// UploadToGitHub uploads a file to GitHub using the browser API
func UploadToGitHub(data []byte, filename string, token string, session string) (string, error) {
	// Step 1: Get upload policy
	policy, err := getGitHubUploadPolicy(filename, len(data), token, session)
	if err != nil {
		return "", fmt.Errorf("failed to get upload policy: %w", err)
	}

	// Step 2: Upload file to S3
	uploadedAsset, err := uploadToGitHubS3(policy, data, filename)
	if err != nil {
		return "", fmt.Errorf("failed to upload to S3: %w", err)
	}

	// Step 3: Return the GitHub asset URL
	return uploadedAsset.Href, nil
}

// getGitHubUploadPolicy gets the upload policy from GitHub
func getGitHubUploadPolicy(filename string, size int, token string, session string) (*GitHubUploadPolicy, error) {
	url := "https://github.com/upload/policies/assets"

	// Prepare request body
	requestBody := map[string]interface{}{
		"name":         filename,
		"size":         size,
		"content_type": getContentType(filename),
		"repository_id": nil, // This might need to be set for private repos
		"repository_nwo": nil,
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}

	// Set headers to mimic browser request
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
	req.Header.Set("Origin", "https://github.com")
	req.Header.Set("Referer", "https://github.com")
	
	// Add authentication
	if session != "" {
		// Use the session cookie for authentication
		req.Header.Set("Cookie", fmt.Sprintf("user_session=%s", session))
		fmt.Printf("[AUTH] Using GitHub session for upload authentication\n")
	} else if token != "" {
		// Fallback to token if no session provided
		req.Header.Set("Authorization", "token "+token)
		fmt.Printf("[AUTH] Using GitHub token for upload (may not work)\n")
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("failed to get upload policy: status %d, body: %s", resp.StatusCode, string(body))
	}

	var policy GitHubUploadPolicy
	if err := json.Unmarshal(body, &policy); err != nil {
		return nil, fmt.Errorf("failed to parse policy response: %w", err)
	}

	return &policy, nil
}

// uploadToGitHubS3 uploads the file to GitHub's S3 bucket
func uploadToGitHubS3(policy *GitHubUploadPolicy, data []byte, filename string) (*GitHubUploadResponse, error) {
	// Create multipart form data
	body := &bytes.Buffer{}
	
	// GitHub uses a specific format for the upload
	// The form data from the policy needs to be included
	boundary := "----WebKitFormBoundary" + generateBoundary()
	
	// Add all form fields from policy
	for key, value := range policy.FormData {
		body.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		body.WriteString(fmt.Sprintf("Content-Disposition: form-data; name=\"%s\"\r\n\r\n", key))
		body.WriteString(fmt.Sprintf("%s\r\n", value))
	}

	// Add authenticity token
	if policy.UploadAuthenticityToken != "" {
		body.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		body.WriteString("Content-Disposition: form-data; name=\"authenticity_token\"\r\n\r\n")
		body.WriteString(fmt.Sprintf("%s\r\n", policy.UploadAuthenticityToken))
	}

	// Add the file
	body.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	body.WriteString(fmt.Sprintf("Content-Disposition: form-data; name=\"file\"; filename=\"%s\"\r\n", filename))
	body.WriteString(fmt.Sprintf("Content-Type: %s\r\n\r\n", getContentType(filename)))
	body.Write(data)
	body.WriteString("\r\n")
	body.WriteString(fmt.Sprintf("--%s--\r\n", boundary))

	req, err := http.NewRequest("POST", policy.UploadURL, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", fmt.Sprintf("multipart/form-data; boundary=%s", boundary))
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
	req.Header.Set("Origin", "https://github.com")
	req.Header.Set("Referer", "https://github.com")

	client := &http.Client{
		Timeout: 60 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // Don't follow redirects
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// GitHub returns a redirect on successful upload
	if resp.StatusCode == http.StatusSeeOther || resp.StatusCode == http.StatusFound {
		// The Location header contains the asset URL
		location := resp.Header.Get("Location")
		if location != "" {
			// Parse the response from the redirect location
			return &GitHubUploadResponse{
				Href: location,
				URL:  location,
			}, nil
		}
	}

	responseBody, _ := io.ReadAll(resp.Body)
	
	// Try to parse as JSON response
	var uploadResp GitHubUploadResponse
	if err := json.Unmarshal(responseBody, &uploadResp); err == nil && uploadResp.Href != "" {
		return &uploadResp, nil
	}

	// If we have the asset info from policy, use it
	if policy.Asset.Href != "" {
		return &GitHubUploadResponse{
			Href: policy.Asset.Href,
			URL:  policy.Asset.Href,
		}, nil
	}

	return nil, fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(responseBody))
}

// getContentType returns the MIME type for a file
func getContentType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	mimeType := mime.TypeByExtension(ext)
	if mimeType == "" {
		// Default to binary
		return "application/octet-stream"
	}
	return mimeType
}

// generateBoundary generates a random boundary for multipart form
func generateBoundary() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// UploadFileToGitHubWithAuth uploads a file using authenticated session
func UploadFileToGitHubWithAuth(data []byte, filename string, owner string, repo string, token string, session string) (string, error) {
	// Alternative approach: Use the GitHub API v3 to create a blob and tree
	// This is more complex but might work better
	
	// First, try the browser API approach with session
	url, err := UploadToGitHub(data, filename, token, session)
	if err == nil {
		return url, nil
	}

	// Fallback: Create a gist or use releases API
	fmt.Printf("[WARNING] Browser API upload failed: %v\n", err)
	fmt.Printf("[INFO] Files will remain on original platform\n")
	
	return "", fmt.Errorf("GitHub upload not available via API")
}

// GitHubAuthenticatedUpload represents a more robust upload approach
type GitHubAuthenticatedUpload struct {
	Token    string
	Owner    string
	Repo     string
	IssueNum int
}

// UploadAttachment uploads an attachment to GitHub
func (g *GitHubAuthenticatedUpload) UploadAttachment(data []byte, filename string) (string, error) {
	// This would need to:
	// 1. Authenticate properly with GitHub
	// 2. Get CSRF tokens if needed
	// 3. Upload to the correct endpoint
	// 4. Return the attachment URL
	
	// For now, we'll return an error indicating this needs browser automation
	return "", fmt.Errorf("GitHub file upload requires browser session authentication")
}

// Note: The GitHub upload API is not officially documented and requires:
// 1. Valid session cookies (not just API token)
// 2. CSRF tokens
// 3. Specific headers that match browser behavior
// 
// For production use, consider:
// - Using GitHub Actions to upload files
// - Creating a GitHub App with proper permissions
// - Using the Releases API to attach files
// - Keeping files on the original platform with links