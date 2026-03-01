// Package threads extracts and downloads videos from Threads posts.
//
// Threads embeds video data in SSR JSON inside the HTML page.
// Required headers: Sec-Fetch-Mode: navigate, Sec-Fetch-Dest: document.
// Fallback: /embed endpoint with a simple <video><source> tag.
package threads

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	utls "github.com/refraction-networking/utls"
)

// video represents an extracted video with its metadata.
type video struct {
	URL     string
	Caption string
	Author  string
}

// client fetches Threads pages and extracts video download URLs.
// It uses uTLS to mimic a real Chrome TLS fingerprint, which is
// required to bypass Meta's bot detection on Threads.
// A separate plain HTTP client is used for CDN video downloads,
// because the CDN does not require TLS fingerprint spoofing and
// may reject the modified ClientHello.
type client struct {
	pageClient  *http.Client // uTLS-based, for threads.com pages
	videoClient *http.Client // standard, for CDN video downloads
	userAgent   string
}

func newClient(userAgent string) *client {
	dialer := &net.Dialer{Timeout: 30 * time.Second}
	transport := &http.Transport{
		DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			conn, err := dialer.DialContext(ctx, network, addr)
			if err != nil {
				return nil, err
			}
			host, _, _ := net.SplitHostPort(addr)

			tlsConn := utls.UClient(conn, &utls.Config{ServerName: host}, utls.HelloChrome_Auto)

			// Build the Chrome ClientHello, then override ALPN to http/1.1
			// so Go's http.Transport (which only supports HTTP/1.x via
			// DialTLSContext) doesn't choke on an HTTP/2 response.
			if err := tlsConn.BuildHandshakeState(); err != nil {
				_ = conn.Close()
				return nil, err
			}
			for _, ext := range tlsConn.Extensions {
				if alpn, ok := ext.(*utls.ALPNExtension); ok {
					alpn.AlpnProtocols = []string{"http/1.1"}
					break
				}
			}
			if err := tlsConn.MarshalClientHello(); err != nil {
				_ = conn.Close()
				return nil, err
			}

			if err := tlsConn.HandshakeContext(ctx); err != nil {
				_ = conn.Close()
				return nil, err
			}
			return tlsConn, nil
		},
	}
	return &client{
		pageClient: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
		videoClient: &http.Client{Timeout: 30 * time.Second},
		userAgent:   userAgent,
	}
}

// extract fetches the Threads post page and returns found videos.
func (c *client) extract(ctx context.Context, postURL string) ([]video, error) {
	postURL, err := normalizeURL(postURL)
	if err != nil {
		return nil, fmt.Errorf("threads: invalid URL: %w", err)
	}

	// Primary: parse SSR data from the main page.
	videos, err := c.extractFromMainPage(ctx, postURL)
	if err == nil && len(videos) > 0 {
		return videos, nil
	}

	// Fallback: parse the embed page.
	videos, embedErr := c.extractFromEmbed(ctx, postURL)
	if embedErr == nil && len(videos) > 0 {
		return videos, nil
	}

	if err != nil {
		return nil, fmt.Errorf("threads: main page extraction failed: %w", err)
	}
	if embedErr != nil {
		return nil, fmt.Errorf("threads: embed extraction failed: %w", embedErr)
	}
	return nil, fmt.Errorf("threads: no video found in post")
}

func (c *client) extractFromMainPage(ctx context.Context, postURL string) ([]video, error) {
	body, err := c.fetchPage(ctx, postURL)
	if err != nil {
		return nil, fmt.Errorf("fetch page: %w", err)
	}
	return parseSSRData(body)
}

func (c *client) extractFromEmbed(ctx context.Context, postURL string) ([]video, error) {
	embedURL := strings.TrimRight(postURL, "/") + "/embed"
	body, err := c.fetchPage(ctx, embedURL)
	if err != nil {
		return nil, fmt.Errorf("fetch embed: %w", err)
	}
	return parseEmbedPage(body)
}

// fetchPage performs an HTTP GET with browser-like headers required by Threads.
func (c *client) fetchPage(ctx context.Context, pageURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-User", "?1")

	resp, err := c.pageClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	// Meta redirects blocked requests to /?error=invalid_post with 200 OK.
	if resp.Request != nil && resp.Request.URL.Query().Get("error") != "" {
		return "", fmt.Errorf("blocked by Meta (redirected to %s)", resp.Request.URL.String())
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// downloadVideo streams the video content from the given URL to w.
func (c *client) downloadVideo(ctx context.Context, videoURL string, w io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, videoURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.videoClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("video download failed with status %d", resp.StatusCode)
	}

	_, err = io.Copy(w, resp.Body)
	return err
}

// --- URL handling ---

// normalizeURL validates and normalizes a Threads post URL to a canonical form.
func normalizeURL(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)

	if !strings.Contains(rawURL, "://") {
		rawURL = "https://" + rawURL
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("cannot parse URL: %w", err)
	}

	host := strings.ToLower(parsed.Hostname())
	if host != "www.threads.com" && host != "threads.com" &&
		host != "www.threads.net" && host != "threads.net" {
		return "", fmt.Errorf("not a Threads URL: %s", host)
	}

	path := strings.TrimRight(parsed.Path, "/")

	// Preserve xmt query param — Meta may use the share token as a
	// legitimacy signal.
	query := ""
	if xmt := parsed.Query().Get("xmt"); xmt != "" {
		query = "?xmt=" + url.QueryEscape(xmt)
	}

	// Short URL format /t/CODE — keep as-is, HTTP client follows redirects.
	if strings.HasPrefix(path, "/t/") {
		return fmt.Sprintf("https://www.threads.com%s%s", path, query), nil
	}

	// /@user/post/CODE format.
	parts := strings.Split(path, "/")
	if len(parts) < 4 || parts[2] != "post" || !strings.HasPrefix(parts[1], "@") {
		return "", fmt.Errorf("unexpected URL path: %s", path)
	}

	username := parts[1]
	code := parts[3]
	return fmt.Sprintf("https://www.threads.com/%s/post/%s%s", username, code, query), nil
}

// --- SSR data parsing ---

type ssrPost struct {
	Code          string         `json:"code"`
	MediaType     int            `json:"media_type"`
	VideoVersions []videoVersion `json:"video_versions"`
	CarouselMedia []carouselItem `json:"carousel_media"`
	Caption       *caption       `json:"caption"`
	User          *user          `json:"user"`
}

type videoVersion struct {
	Type int    `json:"type"`
	URL  string `json:"url"`
}

type carouselItem struct {
	MediaType     int            `json:"media_type"`
	VideoVersions []videoVersion `json:"video_versions"`
}

type caption struct {
	Text string `json:"text"`
}

type user struct {
	Username string `json:"username"`
}

type edgesContainer struct {
	Edges []edge `json:"edges"`
}

type edge struct {
	Node node `json:"node"`
}

type node struct {
	ThreadItems []threadItem `json:"thread_items"`
}

type threadItem struct {
	Post ssrPost `json:"post"`
}

// parseSSRData extracts video information from the Threads SSR HTML.
func parseSSRData(html string) ([]video, error) {
	const marker = "BarcelonaPostPage"
	idx := strings.Index(html, marker)
	if idx == -1 {
		return nil, fmt.Errorf("BarcelonaPostPage marker not found in HTML")
	}

	const edgesMarker = `"edges":[`
	edgesIdx := strings.Index(html[idx:], edgesMarker)
	if edgesIdx == -1 {
		return nil, fmt.Errorf(`"edges" array not found`)
	}
	edgesIdx += idx

	arrayStart := edgesIdx + len(edgesMarker) - 1
	arrayEnd, err := findMatchingBracket(html, arrayStart)
	if err != nil {
		return nil, fmt.Errorf("find edges array end: %w", err)
	}

	edgesJSON := `{"edges":` + html[arrayStart:arrayEnd+1] + `}`
	var container edgesContainer
	if err := json.Unmarshal([]byte(edgesJSON), &container); err != nil {
		return nil, fmt.Errorf("parse edges JSON: %w", err)
	}

	if len(container.Edges) == 0 {
		return nil, fmt.Errorf("empty edges array")
	}
	firstNode := container.Edges[0].Node
	if len(firstNode.ThreadItems) == 0 {
		return nil, fmt.Errorf("empty thread_items")
	}

	post := firstNode.ThreadItems[0].Post
	return extractVideosFromPost(post)
}

func extractVideosFromPost(post ssrPost) ([]video, error) {
	captionText := ""
	if post.Caption != nil {
		captionText = post.Caption.Text
	}
	author := ""
	if post.User != nil {
		author = post.User.Username
	}

	switch post.MediaType {
	case 2:
		videoURL := bestVideoURL(post.VideoVersions)
		if videoURL == "" {
			return nil, fmt.Errorf("video post has no video_versions")
		}
		return []video{{URL: videoURL, Caption: captionText, Author: author}}, nil

	case 8:
		var videos []video
		for _, item := range post.CarouselMedia {
			if item.MediaType == 2 && len(item.VideoVersions) > 0 {
				videos = append(videos, video{
					URL:     bestVideoURL(item.VideoVersions),
					Caption: captionText,
					Author:  author,
				})
			}
		}
		if len(videos) == 0 {
			return nil, fmt.Errorf("carousel post has no video items")
		}
		return videos, nil

	default:
		if len(post.VideoVersions) > 0 {
			return []video{{URL: bestVideoURL(post.VideoVersions), Caption: captionText, Author: author}}, nil
		}
		return nil, fmt.Errorf("post media_type %d has no video", post.MediaType)
	}
}

func bestVideoURL(versions []videoVersion) string {
	if len(versions) == 0 {
		return ""
	}
	for _, v := range versions {
		if v.Type == 101 {
			return v.URL
		}
	}
	return versions[0].URL
}

// findMatchingBracket finds the position of the matching closing bracket.
func findMatchingBracket(s string, start int) (int, error) {
	if start >= len(s) {
		return -1, fmt.Errorf("start position out of bounds")
	}

	open := s[start]
	var close byte
	switch open {
	case '[':
		close = ']'
	case '{':
		close = '}'
	default:
		return -1, fmt.Errorf("expected [ or { at position %d, got %c", start, open)
	}

	depth := 0
	inString := false
	escaped := false

	for i := start; i < len(s); i++ {
		ch := s[i]

		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && inString {
			escaped = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch ch {
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return i, nil
			}
		}
	}
	return -1, fmt.Errorf("no matching bracket found")
}

// --- Embed page parsing ---

var embedSourceRe = regexp.MustCompile(`<source\s+src="([^"]+)"`)

func parseEmbedPage(html string) ([]video, error) {
	matches := embedSourceRe.FindStringSubmatch(html)
	if len(matches) < 2 {
		return nil, fmt.Errorf("no <source> tag found in embed page")
	}
	videoURL := strings.ReplaceAll(matches[1], "&amp;", "&")
	return []video{{URL: videoURL}}, nil
}
