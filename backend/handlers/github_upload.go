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
	UploadURL string            `json:"upload_url"`
	Header    map[string]string `json:"header"`
	Asset     struct {
		ID           int    `json:"id"`
		Name         string `json:"name"`
		Size         int    `json:"size"`
		ContentType  string `json:"content_type"`
		Href         string `json:"href"`
		OriginalName string `json:"original_name"`
	} `json:"asset"`
	// Form can contain different fields depending on the upload type
	// For issue attachments: includes Cache-Control and x-amz-meta-Surrogate-Control
	// For repository files: only includes basic S3 fields
	Form                         map[string]string `json:"form"`
	SameOrigin                   bool              `json:"same_origin"`
	AssetUploadURL               string            `json:"asset_upload_url"`
	UploadAuthenticityToken      string            `json:"upload_authenticity_token"`
	AssetUploadAuthenticityToken string            `json:"asset_upload_authenticity_token"`
}

// GitHubUploadResponse represents the response after uploading
type GitHubUploadResponse struct {
	ID           string `json:"id"`
	URL          string `json:"url"`
	Href         string `json:"href"`
	OriginalName string `json:"original_name"`
}

// UploadToGitHub uploads a file to GitHub using the browser API
// Note: repositoryID should be passed if uploading to a specific repo
func UploadToGitHub(data []byte, filename string, token string, session string) (string, error) {
	return UploadToGitHubWithRepo(data, filename, token, session, "")
}

// UploadToGitHubWithRepo uploads a file to GitHub with a specific repository ID
func UploadToGitHubWithRepo(data []byte, filename string, token string, session string, repositoryID string) (string, error) {
	return UploadToGitHubWithRepoAndReferer(data, filename, token, session, repositoryID, "")
}

// UploadToGitHubWithRepoAndReferer uploads a file to GitHub with a specific repository ID and referer URL
func UploadToGitHubWithRepoAndReferer(data []byte, filename string, token string, session string, repositoryID string, refererURL string) (string, error) {
	// Step 1: Get upload policy
	policy, err := getGitHubUploadPolicy(filename, len(data), token, session, repositoryID)
	if err != nil {
		return "", fmt.Errorf("failed to get upload policy: %w", err)
	}

	// Sometimes GitHub returns the asset URL immediately for small files
	// But we still need to complete the upload process

	// Step 2: Upload file to S3
	s3Response, err := uploadToGitHubS3(policy, data, filename)
	if err != nil {
		fmt.Printf("[ERROR] S3 upload failed: %v\n", err)
		// If S3 upload fails and we don't have an asset URL, we can't continue
		if policy.Asset.Href == "" {
			return "", fmt.Errorf("failed to upload to S3 and no asset URL available: %w", err)
		}
		// If we have an asset URL from the policy, we might still try to use it
		// but the file won't actually be available
		fmt.Printf("[WARNING] S3 upload failed but policy has asset URL: %s\n", policy.Asset.Href)
		fmt.Printf("[WARNING] File may not be accessible at this URL\n")
	} else if s3Response != nil {
		fmt.Printf("[SUCCESS] S3 upload completed, response URL: %s\n", s3Response.Href)
	}

	// Step 3: Confirm the asset upload with GitHub
	if policy.Asset.ID > 0 {
		// Use the provided referer URL or default to GitHub root
		if refererURL == "" {
			refererURL = "https://github.com/"
		}
		err = confirmGitHubAssetUpload(policy.Asset.ID, policy.AssetUploadAuthenticityToken, session, refererURL)
		if err != nil {
			fmt.Printf("[WARNING] Failed to confirm asset upload: %v\n", err)
			// Continue anyway as the upload might still work
		}
	}

	// Return the GitHub asset URL with metadata embedded for proper formatting
	if policy.Asset.Href != "" {
		// Log the original filename for debugging
		fmt.Printf("[UPLOAD] Asset uploaded with original filename: %s\n", filename)
		fmt.Printf("[UPLOAD] Asset URL: %s\n", policy.Asset.Href)
		return policy.Asset.Href, nil
	}

	return "", fmt.Errorf("upload completed but no asset URL received")
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

	// Log form fields for debugging
	fmt.Printf("[UPLOAD] Form fields received:\n")
	for key, value := range policy.Form {
		// Truncate long values for readability
		displayValue := value
		if len(value) > 50 {
			displayValue = value[:50] + "..."
		}
		fmt.Printf("[UPLOAD]     - %s: %s\n", key, displayValue)
	}

	return &policy, nil
}

// uploadToGitHubS3 uploads the file to GitHub's S3 bucket
func uploadToGitHubS3(policy *GitHubUploadPolicy, data []byte, filename string) (*GitHubUploadResponse, error) {
	fmt.Printf("[S3] Starting S3 upload for file: %s\n", filename)
	fmt.Printf("[S3] Upload URL: %s\n", policy.UploadURL)
	fmt.Printf("[S3] Asset already has href: %s\n", policy.Asset.Href)

	// Even if GitHub provided an asset URL, we still need to upload the actual file data
	// The asset URL is just a placeholder until the file is uploaded
	fmt.Printf("[S3] Asset URL provided by GitHub: %s\n", policy.Asset.Href)

	// Check if we have an upload URL
	if policy.UploadURL == "" {
		fmt.Printf("[ERROR] No upload URL provided by GitHub - policy request may have failed\n")
		return nil, fmt.Errorf("no upload URL provided by GitHub")
	}

	fmt.Printf("[S3] Proceeding with S3 upload to: %s\n", policy.UploadURL)

	// Create multipart form data exactly as shown in the curl command
	boundary := "----WebKitFormBoundary" + generateBoundary()
	body := &bytes.Buffer{}

	// Check which fields we actually have
	fmt.Printf("[S3] Form fields available: %d\n", len(policy.Form))
	for key := range policy.Form {
		fmt.Printf("[S3]   - %s\n", key)
	}

	// Define the standard order for S3 form fields
	// Different upload types may have different fields
	standardFields := []string{
		"key",
		"acl",
		"policy",
		"X-Amz-Algorithm",
		"X-Amz-Credential",
		"X-Amz-Date",
		"X-Amz-Signature",
		"Content-Type",
		"Cache-Control",
		"x-amz-meta-Surrogate-Control",
	}

	// Add all fields in the standard order if they exist
	for _, fieldName := range standardFields {
		if val, ok := policy.Form[fieldName]; ok {
			body.WriteString("------" + boundary + "\r\n")
			body.WriteString(fmt.Sprintf("Content-Disposition: form-data; name=\"%s\"\r\n\r\n", fieldName))
			body.WriteString(val + "\r\n")

			// Log critical fields for debugging (but truncate sensitive data)
			if fieldName == "key" || fieldName == "Content-Type" {
				fmt.Printf("[S3] Form field %s: %s\n", fieldName, val)
			} else if fieldName == "X-Amz-Signature" || fieldName == "policy" {
				if len(val) > 20 {
					fmt.Printf("[S3] Form field %s: %s... (truncated)\n", fieldName, val[:20])
				}
			}
		}
	}

	// Get content type from form fields
	contentType := policy.Form["Content-Type"]
	if contentType == "" {
		// Fallback to detecting from filename if not in form
		contentType = getContentType(filename)
	}

	// 11. File (last)
	body.WriteString("------" + boundary + "\r\n")
	body.WriteString(fmt.Sprintf("Content-Disposition: form-data; name=\"file\"; filename=\"%s\"\r\n", filename))
	body.WriteString(fmt.Sprintf("Content-Type: %s\r\n\r\n", contentType))
	body.Write(data)
	body.WriteString("\r\n------" + boundary + "--\r\n")

	// Create request with exact headers from curl
	req, err := http.NewRequest("POST", policy.UploadURL, body)
	if err != nil {
		return nil, err
	}

	// Set headers exactly as in curl command
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Content-Type", fmt.Sprintf("multipart/form-data; boundary=----%s", boundary))
	req.Header.Set("Origin", "https://github.com")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Referer", "https://github.com/")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36")
	req.Header.Set("sec-ch-ua", `"Chromium";v="140", "Not=A?Brand";v="24", "Google Chrome";v="140"`)
	req.Header.Set("sec-ch-ua-mobile", "?0")
	req.Header.Set("sec-ch-ua-platform", `"macOS"`)

	// Log request details for debugging
	fmt.Printf("[S3] Sending request to: %s\n", policy.UploadURL)
	fmt.Printf("[S3] Content-Type: %s\n", req.Header.Get("Content-Type"))
	fmt.Printf("[S3] Body size: %d bytes\n", body.Len())

	client := &http.Client{
		Timeout: 60 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // Don't follow redirects
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("[S3] Request failed: %v\n", err)
		return nil, err
	}
	defer resp.Body.Close()

	fmt.Printf("[S3] Response status: %d\n", resp.StatusCode)
	fmt.Printf("[S3] Response headers: %v\n", resp.Header)

	responseBody, _ := io.ReadAll(resp.Body)

	// GitHub returns 204 No Content on successful upload
	if resp.StatusCode == http.StatusNoContent {
		fmt.Printf("[S3] Upload successful (204 No Content)\n")
		// Return the asset URL from the policy
		if policy.Asset.Href != "" {
			fmt.Printf("[S3] Returning asset URL from policy: %s\n", policy.Asset.Href)
			return &GitHubUploadResponse{
				Href:         policy.Asset.Href,
				URL:          policy.Asset.Href,
				OriginalName: policy.Asset.OriginalName,
			}, nil
		} else {
			fmt.Printf("[WARNING] S3 upload succeeded but no asset URL available\n")
			return nil, fmt.Errorf("upload succeeded but no asset URL available")
		}
	}

	// GitHub returns a redirect on successful upload (303 See Other or 302 Found)
	if resp.StatusCode == http.StatusSeeOther || resp.StatusCode == http.StatusFound {
		location := resp.Header.Get("Location")
		fmt.Printf("[S3] Redirect response received (status %d)\n", resp.StatusCode)
		fmt.Printf("[S3] Redirect location: %s\n", location)

		// Use the asset URL from policy if redirect location is empty
		if location == "" && policy.Asset.Href != "" {
			fmt.Printf("[S3] No redirect location, using asset URL from policy: %s\n", policy.Asset.Href)
			return &GitHubUploadResponse{
				Href:         policy.Asset.Href,
				URL:          policy.Asset.Href,
				OriginalName: policy.Asset.OriginalName,
			}, nil
		} else if location != "" {
			return &GitHubUploadResponse{
				Href: location,
				URL:  location,
			}, nil
		}
	}

	// Try to parse as JSON response
	var uploadResp GitHubUploadResponse
	if err := json.Unmarshal(responseBody, &uploadResp); err == nil && uploadResp.Href != "" {
		fmt.Printf("[S3] Got JSON response with href: %s\n", uploadResp.Href)
		return &uploadResp, nil
	}

	// Log error details
	fmt.Printf("[S3] Upload failed with status %d\n", resp.StatusCode)
	fmt.Printf("[S3] Response body: %s\n", string(responseBody))

	return nil, fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(responseBody))
}

// confirmGitHubAssetUpload confirms the asset upload with GitHub
func confirmGitHubAssetUpload(assetID int, authenticityToken string, session string, refererURL string) error {
	// Use the correct endpoint for asset confirmation - repository-files endpoint
	url := fmt.Sprintf("https://github.com/upload/repository-files/%d", assetID)

	fmt.Printf("[CONFIRM] Confirming asset upload for ID: %d\n", assetID)
	fmt.Printf("[CONFIRM] Using URL: %s\n", url)
	fmt.Printf("[CONFIRM] Using referer: %s\n", refererURL)

	// Log authenticity token safely
	if len(authenticityToken) > 20 {
		fmt.Printf("[CONFIRM] Authenticity token: %s...\n", authenticityToken[:20])
	} else {
		fmt.Printf("[CONFIRM] Authenticity token length: %d\n", len(authenticityToken))
	}

	// Create multipart form data with authenticity token
	boundary := "----WebKitFormBoundary" + generateBoundary()
	body := &bytes.Buffer{}

	// Add authenticity token
	body.WriteString("------" + boundary + "\r\n")
	body.WriteString("Content-Disposition: form-data; name=\"authenticity_token\"\r\n\r\n")
	body.WriteString(authenticityToken + "\r\n")
	body.WriteString("------" + boundary + "--\r\n")

	req, err := http.NewRequest("PUT", url, body)
	if err != nil {
		return fmt.Errorf("failed to create confirm request: %w", err)
	}

	// Set headers exactly as in the curl command
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Content-Type", fmt.Sprintf("multipart/form-data; boundary=----%s", boundary))
	req.Header.Set("Origin", "https://github.com")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Priority", "u=1, i")
	req.Header.Set("Referer", refererURL)
	req.Header.Set("Sec-Ch-Ua", `"Chromium";v="140", "Not=A?Brand";v="24", "Google Chrome";v="140"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", `"macOS"`)
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("x-github-client-version", "470543cfe11ca9768bc6453923bfd6aa1a3adf6b")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	// Generate a nonce
	nonce := fmt.Sprintf("v2:%s", generateNonce())
	req.Header.Set("X-Fetch-Nonce", nonce)

	// Set GitHub client version (this might be important)
	req.Header.Set("X-Github-Client-Version", "4414191ff04d0af5c0143d435276147f92c9264f")

	// Add session cookie
	if session != "" {
		cookieHeader := fmt.Sprintf("user_session=%s; __Host-user_session_same_site=%s; logged_in=yes", session, session)
		req.Header.Set("Cookie", cookieHeader)
		fmt.Printf("[CONFIRM] Using session cookie for confirmation\n")
	} else {
		fmt.Printf("[WARNING] No session cookie for asset confirmation - this will likely fail\n")
		return fmt.Errorf("session cookie required for asset confirmation")
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	fmt.Printf("[CONFIRM] Sending PUT request to: %s\n", url)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("confirm request failed: %w", err)
	}
	defer resp.Body.Close()

	fmt.Printf("[CONFIRM] Response status: %d\n", resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)

	// Check for success (usually 200 or 204)
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent {
		fmt.Printf("[CONFIRM] Asset upload confirmed successfully\n")
		return nil
	}

	// Log response for debugging
	fmt.Printf("[CONFIRM] Response body: %s\n", string(respBody))
	fmt.Printf("[CONFIRM] Response headers: %v\n", resp.Header)

	// Parse JSON error response if available
	var errorResp map[string]interface{}
	if err := json.Unmarshal(respBody, &errorResp); err == nil {
		fmt.Printf("[CONFIRM] Parsed error response: %+v\n", errorResp)
		if msg, ok := errorResp["message"].(string); ok {
			// Check for specific error messages
			if strings.Contains(msg, "Invalid Asset") {
				fmt.Printf("[CONFIRM] Asset may have already been confirmed or is invalid\n")
				// Don't fail on "Invalid Asset" - the upload might still work
				return nil
			}
			return fmt.Errorf("confirmation failed: %s", msg)
		}
	}

	// If status is 422 (Unprocessable Entity), it might mean the asset is already confirmed
	if resp.StatusCode == 422 {
		fmt.Printf("[CONFIRM] Got 422 status - asset may already be confirmed\n")
		return nil
	}

	return fmt.Errorf("confirmation failed with status %d: %s", resp.StatusCode, string(respBody))
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
