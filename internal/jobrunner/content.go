package jobrunner

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-shiori/go-readability"
)

var (
	// User agent to use for fetching articles
	userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

	// Regex to clean up excessive whitespace
	excessiveNewlines = regexp.MustCompile(`\n{3,}`)
	excessiveSpaces   = regexp.MustCompile(` {2,}`)
)

// FetchArticleContent fetches and extracts readable content from a URL.
func FetchArticleContent(ctx context.Context, url string) (string, error) {
	if url == "" {
		return "(No URL provided)", nil
	}

	// Create request with timeout
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	client := &http.Client{
		Timeout: 20 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	// Limit response size to 5MB
	limitedReader := io.LimitReader(resp.Body, 5*1024*1024)

	// Use go-readability to extract main content
	article, err := readability.FromReader(limitedReader, nil)
	if err != nil {
		return "", fmt.Errorf("parse content: %w", err)
	}

	content := article.TextContent
	if content == "" {
		return "[Content could not be extracted from this page]", nil
	}

	// Clean up whitespace
	content = excessiveNewlines.ReplaceAllString(content, "\n\n")
	content = excessiveSpaces.ReplaceAllString(content, " ")
	content = strings.TrimSpace(content)

	return content, nil
}
