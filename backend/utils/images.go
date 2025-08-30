package utils

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

var imageRegex = regexp.MustCompile(`!\[([^\]]*)\]\(([^)]+)\)`)
var htmlImageRegex = regexp.MustCompile(`<img[^>]+src="([^"]+)"[^>]*>`)

type ImageProcessor struct {
	client *http.Client
}

func NewImageProcessor() *ImageProcessor {
	return &ImageProcessor{
		client: &http.Client{},
	}
}

func (p *ImageProcessor) DownloadImage(url string) ([]byte, string, error) {
	resp, err := p.client.Get(url)
	if err != nil {
		return nil, "", fmt.Errorf("failed to download image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("failed to download image: status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read image data: %w", err)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "image/png"
	}

	return data, contentType, nil
}

func (p *ImageProcessor) ExtractImageURLs(content string) []string {
	var urls []string
	seen := make(map[string]bool)

	// Extract markdown images
	matches := imageRegex.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) > 2 {
			url := match[2]
			if !seen[url] && (strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")) {
				urls = append(urls, url)
				seen[url] = true
			}
		}
	}

	// Extract HTML img tags
	htmlMatches := htmlImageRegex.FindAllStringSubmatch(content, -1)
	for _, match := range htmlMatches {
		if len(match) > 1 {
			url := match[1]
			if !seen[url] && (strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")) {
				urls = append(urls, url)
				seen[url] = true
			}
		}
	}

	return urls
}

func (p *ImageProcessor) ReplaceImageURLs(content string, urlMap map[string]string) string {
	result := content

	// Replace markdown images
	result = imageRegex.ReplaceAllStringFunc(result, func(match string) string {
		parts := imageRegex.FindStringSubmatch(match)
		if len(parts) > 2 {
			oldURL := parts[2]
			if newURL, ok := urlMap[oldURL]; ok {
				return fmt.Sprintf("![%s](%s)", parts[1], newURL)
			}
		}
		return match
	})

	// Replace HTML img tags
	result = htmlImageRegex.ReplaceAllStringFunc(result, func(match string) string {
		parts := htmlImageRegex.FindStringSubmatch(match)
		if len(parts) > 1 {
			oldURL := parts[1]
			if newURL, ok := urlMap[oldURL]; ok {
				return strings.Replace(match, oldURL, newURL, 1)
			}
		}
		return match
	})

	return result
}

func GetFilenameFromURL(url string) string {
	parts := strings.Split(url, "/")
	if len(parts) > 0 {
		filename := parts[len(parts)-1]
		// Remove query parameters
		if idx := strings.Index(filename, "?"); idx != -1 {
			filename = filename[:idx]
		}
		if filename == "" {
			return "image.png"
		}
		return filename
	}
	return "image.png"
}

func IsImageURL(url string) bool {
	lowerURL := strings.ToLower(url)
	imageExtensions := []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".bmp"}
	
	for _, ext := range imageExtensions {
		if strings.Contains(lowerURL, ext) {
			return true
		}
	}
	
	// Check for image hosting services
	imageHosts := []string{
		"githubusercontent.com",
		"gitlab.com/uploads",
		"i.imgur.com",
		"camo.githubusercontent.com",
	}
	
	for _, host := range imageHosts {
		if strings.Contains(lowerURL, host) {
			return true
		}
	}
	
	return false
}

type UploadResult struct {
	URL      string
	Markdown string
}

// UploadToGitHub uploads an image to a GitHub issue comment and returns the URL
func UploadToGitHub(imageData []byte, filename string) (*UploadResult, error) {
	// GitHub doesn't have a direct API for image uploads
	// Images are typically uploaded through issue/PR comments
	// This would need to be implemented with a workaround or using a separate service
	return nil, fmt.Errorf("GitHub image upload requires manual implementation or external service")
}

// UploadToGitLab uploads an image to a GitLab project and returns the URL
func UploadToGitLab(projectID int, imageData []byte, filename string, token string, baseURL string) (*UploadResult, error) {
	url := fmt.Sprintf("%s/api/v4/projects/%d/uploads", baseURL, projectID)
	
	body := &bytes.Buffer{}
	body.Write(imageData)
	
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return nil, err
	}
	
	req.Header.Set("PRIVATE-TOKEN", token)
	req.Header.Set("Content-Type", "multipart/form-data")
	
	// Note: This is simplified. In practice, you'd need proper multipart form encoding
	// Using the GitLab client library would be better for this
	
	return nil, fmt.Errorf("GitLab upload requires proper multipart implementation")
}