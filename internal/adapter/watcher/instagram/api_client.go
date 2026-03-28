package instagram

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

const (
	igAppID  = "936619743392459"
	igAPIURL = "https://www.instagram.com"
)

// APIClient fetches story metadata from Instagram's REST API
// to detect reshared (quoted) stories.
type APIClient struct {
	cookiePath func() string
	log        *slog.Logger
	apiURL     string // defaults to igAPIURL; overridden in tests

	mu      sync.RWMutex
	userIDs map[string]string // username → numeric user ID cache
}

func NewAPIClient(cookiePath func() string, log *slog.Logger) *APIClient {
	return &APIClient{
		cookiePath: cookiePath,
		log:        log,
		apiURL:     igAPIURL,
		userIDs:    make(map[string]string),
	}
}

// EnrichStoryMetadata implements domain.StoryMetadataEnricher.
// Returns nil reshare (not an error) when the story is not a reshare.
func (c *APIClient) EnrichStoryMetadata(ctx context.Context, username string, storyID string) (*domain.StoryReshare, error) {
	userID, err := c.resolveUserID(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("resolve user ID for @%s: %w", username, err)
	}

	items, err := c.fetchReelsMedia(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("fetch reels media for @%s: %w", username, err)
	}

	for _, item := range items {
		if !matchStoryID(item, storyID) {
			continue
		}
		if item.StoryReshares == nil || item.StoryReshares.ResharedStory == nil {
			return nil, nil
		}
		rs := item.StoryReshares.ResharedStory
		if rs.User.Username == "" {
			return nil, nil
		}
		return &domain.StoryReshare{
			Username: rs.User.Username,
			StoryID:  rs.ID,
		}, nil
	}

	c.log.Debug("story not found in reels_media response", "username", username, "story_id", storyID)
	return nil, nil
}

func (c *APIClient) resolveUserID(ctx context.Context, username string) (string, error) {
	c.mu.RLock()
	if id, ok := c.userIDs[username]; ok {
		c.mu.RUnlock()
		return id, nil
	}
	c.mu.RUnlock()

	client, err := c.buildHTTPClient()
	if err != nil {
		return "", err
	}

	reqURL := c.apiURL + "/api/v1/users/web_profile_info/?username=" + url.QueryEscape(username)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", err
	}
	setIGHeaders(req)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request user profile: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("user profile API returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	id, err := parseUserID(body)
	if err != nil {
		return "", err
	}

	c.mu.Lock()
	c.userIDs[username] = id
	c.mu.Unlock()

	c.log.Debug("resolved instagram user ID", "username", username, "user_id", id)
	return id, nil
}

func (c *APIClient) fetchReelsMedia(ctx context.Context, userID string) ([]reelsItem, error) {
	client, err := c.buildHTTPClient()
	if err != nil {
		return nil, err
	}

	reqURL := c.apiURL + "/api/v1/feed/reels_media/?reel_ids=" + url.QueryEscape(userID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	setIGHeaders(req)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request reels media: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("reels media API returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	return parseReelsMediaJSON(body)
}

func (c *APIClient) buildHTTPClient() (*http.Client, error) {
	cp := c.cookiePath()
	jar, err := buildCookieJar(cp, c.apiURL)
	if err != nil {
		return nil, fmt.Errorf("building cookie jar: %w", err)
	}
	return &http.Client{
		Timeout: 15 * time.Second,
		Jar:     jar,
	}, nil
}

// --- HTTP helpers ---

func setIGHeaders(req *http.Request) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("X-IG-App-ID", igAppID)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Accept", "*/*")
}

// --- Cookie parsing ---

func buildCookieJar(filePath, baseURL string) (http.CookieJar, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	cookies, err := parseNetscapeCookies(filePath)
	if err != nil {
		return nil, err
	}
	u, _ := url.Parse(baseURL)
	jar.SetCookies(u, cookies)
	return jar, nil
}

func parseNetscapeCookies(filePath string) ([]*http.Cookie, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("opening cookie file: %w", err)
	}
	defer func() { _ = f.Close() }()

	var cookies []*http.Cookie
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 7 {
			continue
		}
		secure := strings.EqualFold(fields[3], "TRUE")
		cookies = append(cookies, &http.Cookie{
			Name:   fields[5],
			Value:  fields[6],
			Domain: fields[0],
			Path:   fields[2],
			Secure: secure,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading cookie file: %w", err)
	}
	return cookies, nil
}

// --- JSON response types ---

type reelsMediaResponse struct {
	ReelsMedia []reelsMediaEntry `json:"reels_media"`
}

type reelsMediaEntry struct {
	Items []reelsItem `json:"items"`
}

type reelsItem struct {
	ID            string         `json:"id"`
	Pk            json.Number    `json:"pk"`
	StoryReshares *storyReshares `json:"story_reshares"`
}

type storyReshares struct {
	ResharedStory *resharedStory `json:"reshared_story"`
}

type resharedStory struct {
	User resharedUser `json:"user"`
	ID   string       `json:"id"`
}

type resharedUser struct {
	Username string `json:"username"`
	Pk       int64  `json:"pk"`
}

type userProfileResponse struct {
	Data struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	} `json:"data"`
}

// --- JSON parsing ---

func parseUserID(data []byte) (string, error) {
	var resp userProfileResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", fmt.Errorf("parsing user profile JSON: %w", err)
	}
	if resp.Data.User.ID == "" {
		return "", fmt.Errorf("user ID not found in profile response")
	}
	return resp.Data.User.ID, nil
}

func parseReelsMediaJSON(data []byte) ([]reelsItem, error) {
	var resp reelsMediaResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing reels media JSON: %w", err)
	}
	if len(resp.ReelsMedia) == 0 {
		return nil, nil
	}
	return resp.ReelsMedia[0].Items, nil
}

// matchStoryID compares yt-dlp story ID with the Instagram API item.
// yt-dlp may return IDs in different formats, so we try multiple matching strategies.
func matchStoryID(item reelsItem, storyID string) bool {
	if item.ID == storyID {
		return true
	}
	if item.Pk.String() == storyID {
		return true
	}
	// yt-dlp sometimes returns ID with timestamp suffix like "12345_67890".
	// Try matching just the numeric pk against the prefix.
	if strings.HasPrefix(storyID, item.Pk.String()+"_") {
		return true
	}
	if strings.HasPrefix(item.ID, storyID+"_") || strings.HasPrefix(storyID, item.ID+"_") {
		return true
	}
	return false
}
