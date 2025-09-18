package handlers

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

// GitHubUploadPolicy represents the response from GitHub's upload policy endpoint
type GitHubUploadPolicy struct {
	UploadURL                   string            `json:"upload_url"`
	Header                      map[string]string `json:"header"`
	Asset                       struct {
		ID           int    `json:"id"`
		Name         string `json:"name"`
		Size         int    `json:"size"`
		ContentType  string `json:"content_type"`
		Href         string `json:"href"`
		OriginalName string `json:"original_name"`
	} `json:"asset"`
	Form                        map[string]string `json:"form"`
	SameOrigin                  bool              `json:"same_origin"`
	AssetUploadURL              string            `json:"asset_upload_url"`
	UploadAuthenticityToken     string            `json:"upload_authenticity_token"`
	AssetUploadAuthenticityToken string           `json:"asset_upload_authenticity_token"`
}

// GitHubUploadResponse represents the response after uploading
type GitHubUploadResponse struct {
	ID          string `json:"id"`
	URL         string `json:"url"`
	Href        string `json:"href"`
	OriginalName string `json:"original_name"`
}

// UploadToGitHub uploads a file to GitHub using the browser API
// Note: repositoryID should be passed if uploading to a specific repo
func UploadToGitHub(data []byte, filename string, token string, session string) (string, error) {
	return UploadToGitHubWithRepo(data, filename, token, session, "")
}

// UploadToGitHubWithRepo uploads a file to GitHub with a specific repository ID
func UploadToGitHubWithRepo(data []byte, filename string, token string, session string, repositoryID string) (string, error) {
	// Step 1: Get upload policy
	policy, err := getGitHubUploadPolicy(filename, len(data), token, session, repositoryID)
	if err != nil {
		return "", fmt.Errorf("failed to get upload policy: %w", err)
	}

	// Check if GitHub already provided the asset URL (sometimes happens for small files)
	if policy.Asset.Href != "" {
		fmt.Printf("[UPLOAD] GitHub already created asset, using URL: %s\n", policy.Asset.Href)
		return policy.Asset.Href, nil
	}

	// Step 2: Upload file to S3 if needed
	uploadedAsset, err := uploadToGitHubS3(policy, data, filename)
	if err != nil {
		return "", fmt.Errorf("failed to upload to S3: %w", err)
	}

	// Step 3: Return the GitHub asset URL
	return uploadedAsset.Href, nil
}

// getGitHubUploadPolicy gets the upload policy from GitHub
func getGitHubUploadPolicy(filename string, size int, token string, session string, repositoryID string) (*GitHubUploadPolicy, error) {
	url := "https://github.com/upload/policies/assets"

	fmt.Printf("[UPLOAD] Requesting upload policy for file: %s (size: %d bytes)\n", filename, size)
	if repositoryID != "" {
		fmt.Printf("[UPLOAD] Repository ID: %s\n", repositoryID)
	}

	// Create multipart form data like the browser does
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add repository_id if provided
	if repositoryID != "" {
		if err := writer.WriteField("repository_id", repositoryID); err != nil {
			return nil, err
		}
	}

	// Add required fields
	if err := writer.WriteField("name", filename); err != nil {
		return nil, err
	}
	if err := writer.WriteField("size", fmt.Sprintf("%d", size)); err != nil {
		return nil, err
	}
	if err := writer.WriteField("content_type", getContentType(filename)); err != nil {
		return nil, err
	}

	if err := writer.Close(); err != nil {
		return nil, err
	}

	fmt.Printf("[UPLOAD] Form data size: %d bytes\n", body.Len())

	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return nil, err
	}

	// Set headers to match the browser request exactly
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Github-Verified-Fetch", "true")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Priority", "u=1, i")
	req.Header.Set("Sec-Ch-Ua", `"Chromium";v="140", "Not=A?Brand";v="24", "Google Chrome";v="140"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", `"macOS"`)
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Origin", "https://github.com")
	req.Header.Set("Referer", "https://github.com/")

	// Generate a nonce like GitHub does
	nonce := fmt.Sprintf("v2:%s", generateNonce())
	req.Header.Set("X-Fetch-Nonce", nonce)

	// Add authentication
	if session != "" {
		// Use the session cookie for authentication
		// GitHub also needs other cookies for CSRF protection
		cookieHeader := fmt.Sprintf("user_session=%s; logged_in=yes", session)
		req.Header.Set("Cookie", cookieHeader)
		fmt.Printf("[AUTH] Using GitHub session for upload authentication\n")
		fmt.Printf("[AUTH] Cookie header length: %d\n", len(cookieHeader))
	} else if token != "" {
		// Fallback to token if no session provided
		req.Header.Set("Authorization", "token "+token)
		fmt.Printf("[AUTH] Using GitHub token for upload (may not work for uploads)\n")
		fmt.Printf("[WARNING] GitHub file uploads typically require session cookies, not just API tokens\n")
	} else {
		fmt.Printf("[WARNING] No authentication provided for GitHub upload\n")
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	fmt.Printf("[UPLOAD] Sending request to GitHub...\n")
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("[ERROR] Request failed: %v\n", err)
		return nil, err
	}
	defer resp.Body.Close()

	fmt.Printf("[UPLOAD] Response status: %d\n", resp.StatusCode)
	fmt.Printf("[UPLOAD] Response headers: %v\n", resp.Header)

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	fmt.Printf("[UPLOAD] Response body length: %d\n", len(respBody))

	// Status 42 might mean "422 Unprocessable Entity" with a typo, or it could be a custom code
	if resp.StatusCode == 422 || resp.StatusCode == 42 {
		fmt.Printf("[ERROR] GitHub returned status %d (Unprocessable Entity)\n", resp.StatusCode)
		fmt.Printf("[ERROR] This usually means:\n")
		fmt.Printf("[ERROR]   1. Invalid or expired session cookie\n")
		fmt.Printf("[ERROR]   2. Missing required parameters\n")
		fmt.Printf("[ERROR]   3. File size or type restrictions\n")
		fmt.Printf("[ERROR] Response: %s\n", string(respBody))
		return nil, fmt.Errorf("GitHub upload not available - session may be invalid or expired")
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		fmt.Printf("[ERROR] GitHub upload policy request failed\n")
		fmt.Printf("[ERROR] Status: %d\n", resp.StatusCode)
		fmt.Printf("[ERROR] Response: %s\n", string(respBody))

		// Check for specific error messages
		if strings.Contains(string(respBody), "browser did something unexpected") {
			fmt.Printf("[ERROR] GitHub detected non-browser request\n")
			fmt.Printf("[INFO] This error typically means:\n")
			fmt.Printf("[INFO]   1. Missing or invalid session cookie\n")
			fmt.Printf("[INFO]   2. Missing CSRF token\n")
			fmt.Printf("[INFO]   3. Request doesn't match browser fingerprint\n")
			return nil, fmt.Errorf("GitHub requires valid browser session for uploads. Please provide 'user_session' cookie value")
		}

		return nil, fmt.Errorf("failed to get upload policy: status %d", resp.StatusCode)
	}

	var policy GitHubUploadPolicy
	if err := json.Unmarshal(respBody, &policy); err != nil {
		return nil, fmt.Errorf("failed to parse policy response: %w", err)
	}

	fmt.Printf("[UPLOAD] Policy response received:\n")
	fmt.Printf("[UPLOAD]   - Asset ID: %d\n", policy.Asset.ID)
	fmt.Printf("[UPLOAD]   - Asset URL: %s\n", policy.Asset.Href)
	fmt.Printf("[UPLOAD]   - Upload URL: %s\n", policy.UploadURL)
	fmt.Printf("[UPLOAD]   - Form fields: %d\n", len(policy.Form))

	return &policy, nil
}

// uploadToGitHubS3 uploads the file to GitHub's S3 bucket
func uploadToGitHubS3(policy *GitHubUploadPolicy, data []byte, filename string) (*GitHubUploadResponse, error) {
	fmt.Printf("[S3] Starting S3 upload for file: %s\n", filename)
	fmt.Printf("[S3] Upload URL: %s\n", policy.UploadURL)
	fmt.Printf("[S3] Asset already has href: %s\n", policy.Asset.Href)

	// GitHub already created the asset and gave us the URL
	// We can return it immediately if it's already uploaded
	if policy.Asset.Href != "" {
		fmt.Printf("[S3] Asset already uploaded, returning existing URL\n")
		return &GitHubUploadResponse{
			Href: policy.Asset.Href,
			URL:  policy.Asset.Href,
			OriginalName: policy.Asset.OriginalName,
		}, nil
	}

	// Create multipart form data
	body := &bytes.Buffer{}

	// GitHub uses a specific format for the upload
	// The form data from the policy needs to be included
	boundary := "----WebKitFormBoundary" + generateBoundary()

	// Add all form fields from policy.Form (not FormData)
	// The order matters - add form fields first, then file
	formFields := []string{"key", "acl", "policy", "X-Amz-Algorithm", "X-Amz-Credential", "X-Amz-Date", "X-Amz-Signature", "Content-Type", "Cache-Control", "x-amz-meta-Surrogate-Control"}

	for _, key := range formFields {
		if value, ok := policy.Form[key]; ok {
			body.WriteString(fmt.Sprintf("--%s\r\n", boundary))
			body.WriteString(fmt.Sprintf("Content-Disposition: form-data; name=\"%s\"\r\n\r\n", key))
			body.WriteString(fmt.Sprintf("%s\r\n", value))
		}
	}

	// Add the file last
	body.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	body.WriteString(fmt.Sprintf("Content-Disposition: form-data; name=\"file\"; filename=\"%s\"\r\n", filename))
	body.WriteString(fmt.Sprintf("Content-Type: %s\r\n\r\n", policy.Form["Content-Type"]))
	body.Write(data)
	body.WriteString("\r\n")
	body.WriteString(fmt.Sprintf("--%s--\r\n", boundary))

	req, err := http.NewRequest("POST", policy.UploadURL, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", fmt.Sprintf("multipart/form-data; boundary=%s", boundary))
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Origin", "https://github.com")
	req.Header.Set("Referer", "https://github.com/")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "cross-site")

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

	// First try standard mime type detection
	mimeType := mime.TypeByExtension(ext)

	// If standard detection fails, use a comprehensive mapping for common image types
	if mimeType == "" {
		switch ext {
		case ".jpg", ".jpeg":
			return "image/jpeg"
		case ".png":
			return "image/png"
		case ".gif":
			return "image/gif"
		case ".bmp":
			return "image/bmp"
		case ".webp":
			return "image/webp"
		case ".svg":
			return "image/svg+xml"
		case ".ico":
			return "image/x-icon"
		case ".tiff", ".tif":
			return "image/tiff"
		case ".heic":
			return "image/heic"
		case ".heif":
			return "image/heif"
		case ".avif":
			return "image/avif"
		case ".pdf":
			return "application/pdf"
		case ".zip":
			return "application/zip"
		case ".tar":
			return "application/x-tar"
		case ".gz":
			return "application/gzip"
		case ".mp4":
			return "video/mp4"
		case ".webm":
			return "video/webm"
		case ".mov":
			return "video/quicktime"
		case ".avi":
			return "video/x-msvideo"
		case ".mp3":
			return "audio/mpeg"
		case ".wav":
			return "audio/wav"
		case ".ogg":
			return "audio/ogg"
		case ".txt":
			return "text/plain"
		case ".json":
			return "application/json"
		case ".xml":
			return "application/xml"
		case ".html", ".htm":
			return "text/html"
		case ".css":
			return "text/css"
		case ".js":
			return "application/javascript"
		case ".ts":
			return "application/typescript"
		default:
			// Only use octet-stream as last resort
			return "application/octet-stream"
		}
	}

	// Clean up the mime type (remove charset info if present)
	if idx := strings.Index(mimeType, ";"); idx != -1 {
		mimeType = mimeType[:idx]
	}

	return strings.TrimSpace(mimeType)
}

// generateBoundary generates a random boundary for multipart form
func generateBoundary() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// generateNonce generates a random nonce like GitHub expects
func generateNonce() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
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