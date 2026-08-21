// Package threadsurl converts any Threads link into a canonical post URL that
// download backends can work with.
//
// The Threads app shares posts as /share/CODE links. Following such a link
// lands on one of two places, depending on how browser-like the client looks:
//
//	https://www.threads.com/@user/post/CODE?xmt=...&slof=1
//	https://www.threads.com/?xmt=...&injected_media_ids=["3954841119435301839"]
//
// Neither form is accepted as-is by the backends: lovethreads.net rejects
// /share/ links outright, and the root URL carries the post only as a numeric
// media id. That id is the shortcode in base64url disguise, so it can be
// converted back into a /t/CODE short link.
//
// Meta does not always stop at the post URL: when the post is deleted,
// private, or viewable only to logged-in users, it redirects one hop further
// to https://www.threads.com/?error=invalid_post. The post URL from the
// middle of the chain is still the answer we want, so every hop is inspected
// and the redirect chain is abandoned as soon as one of them canonicalizes.
package threadsurl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
)

// shortcodeAlphabet is the base64url alphabet Meta uses to encode media ids.
const shortcodeAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"

// defaultUserAgent is used when the caller does not provide one.
const defaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) " +
	"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

// maxRedirects caps how many hops a share link may take before giving up.
const maxRedirects = 10

var (
	// ErrUnsupportedURL means the URL is not a Threads post URL.
	ErrUnsupportedURL = errors.New("threads: unsupported URL")
	// ErrNeedsRedirect means the URL can only be canonicalized by following
	// its HTTP redirect — see Resolve.
	ErrNeedsRedirect = errors.New("threads: URL requires redirect resolution")
	// ErrPostUnavailable means Threads answered with its error page: the post
	// is deleted, private, or viewable only to logged-in users.
	ErrPostUnavailable = errors.New("threads: post is unavailable (deleted, private, or requires login)")
)

// Canonical rewrites a Threads URL to canonical form without touching the
// network. It returns ErrNeedsRedirect for /share/ links.
func Canonical(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)

	if !strings.Contains(rawURL, "://") {
		rawURL = "https://" + rawURL
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("%w: cannot parse %q: %v", ErrUnsupportedURL, rawURL, err)
	}

	host := strings.ToLower(parsed.Hostname())
	if host != "www.threads.com" && host != "threads.com" &&
		host != "www.threads.net" && host != "threads.net" {
		return "", fmt.Errorf("%w: not a Threads host: %s", ErrUnsupportedURL, host)
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
		return "https://www.threads.com" + path + query, nil
	}

	// /@user/post/CODE format.
	parts := strings.Split(path, "/")
	if len(parts) >= 4 && parts[2] == "post" && strings.HasPrefix(parts[1], "@") {
		return fmt.Sprintf("https://www.threads.com/%s/post/%s%s", parts[1], parts[3], query), nil
	}

	// Root URL — the post survives only as injected_media_ids.
	if path == "" {
		if code, ok := shortcodeFromMediaID(mediaIDFromParam(parsed.Query().Get("injected_media_ids"))); ok {
			return "https://www.threads.com/t/" + code + query, nil
		}
		if reason := parsed.Query().Get("error"); reason != "" {
			return "", fmt.Errorf("%w: %s", ErrPostUnavailable, reason)
		}
	}

	// Share links carry an opaque token that only Meta can resolve.
	if strings.HasPrefix(path, "/share/") {
		return "", fmt.Errorf("%w: %s", ErrNeedsRedirect, path)
	}

	return "", fmt.Errorf("%w: unexpected path %q", ErrUnsupportedURL, path)
}

// Resolve canonicalizes rawURL, following the redirect for /share/ links.
// No request is made when the URL can be canonicalized offline.
func Resolve(ctx context.Context, client *http.Client, userAgent, rawURL string) (string, error) {
	canonical, err := Canonical(rawURL)
	if err == nil {
		return canonical, nil
	}
	if !errors.Is(err, ErrNeedsRedirect) {
		return "", err
	}

	visited, err := followRedirects(ctx, client, userAgent, rawURL)
	if err != nil {
		return "", fmt.Errorf("threads: resolve share link: %w", err)
	}

	unavailable := false
	for _, visitedURL := range visited {
		canonical, err = Canonical(visitedURL)
		if err == nil {
			return canonical, nil
		}
		if errors.Is(err, ErrPostUnavailable) {
			unavailable = true
		}
	}

	if unavailable {
		return "", fmt.Errorf("%w: %s", ErrPostUnavailable, rawURL)
	}
	return "", fmt.Errorf("threads: share link resolved to %q: %w", visited[len(visited)-1], err)
}

// followRedirects performs a browser-like GET and reports every URL the
// client was redirected to, in order, ending with the URL it landed on.
// It stops following as soon as a hop can be canonicalized, so the error page
// Meta appends to unavailable posts never overwrites the post URL.
func followRedirects(ctx context.Context, client *http.Client, userAgent, rawURL string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}

	if userAgent == "" {
		userAgent = defaultUserAgent
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-User", "?1")

	var visited []string
	stopped := false

	// Copy the client so the caller keeps its own redirect policy while the
	// transport (uTLS for Threads pages) is reused.
	following := *client
	following.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return errors.New("stopped after too many redirects")
		}
		visited = append(visited, req.URL.String())
		if _, err := Canonical(req.URL.String()); err != nil {
			return nil
		}
		stopped = true
		return http.ErrUseLastResponse
	}

	resp, err := following.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
	}()

	if !stopped {
		if resp.Request == nil || resp.Request.URL == nil {
			return nil, errors.New("no final URL in response")
		}
		visited = append(visited, resp.Request.URL.String())
	}
	return visited, nil
}

// mediaIDFromParam extracts the first id from an injected_media_ids value,
// which is a JSON array such as ["3954841119435301839"].
func mediaIDFromParam(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	if !strings.HasPrefix(raw, "[") {
		if _, ok := new(big.Int).SetString(raw, 10); !ok {
			return ""
		}
		return raw
	}

	var ids []string
	if err := json.Unmarshal([]byte(raw), &ids); err != nil || len(ids) == 0 {
		return ""
	}
	return ids[0]
}

// shortcodeFromMediaID converts a numeric media id into the base64url
// shortcode used in Threads and Instagram post URLs.
func shortcodeFromMediaID(mediaID string) (string, bool) {
	// Ids sometimes come as "<media id>_<user id>" — the first part is ours.
	mediaID, _, _ = strings.Cut(strings.TrimSpace(mediaID), "_")
	if mediaID == "" {
		return "", false
	}

	n, ok := new(big.Int).SetString(mediaID, 10)
	if !ok || n.Sign() <= 0 {
		return "", false
	}

	base := big.NewInt(int64(len(shortcodeAlphabet)))
	rem := new(big.Int)
	var code []byte
	for n.Sign() > 0 {
		n.DivMod(n, base, rem)
		code = append(code, shortcodeAlphabet[rem.Int64()])
	}

	// Digits were produced least-significant first.
	for i, j := 0, len(code)-1; i < j; i, j = i+1, j-1 {
		code[i], code[j] = code[j], code[i]
	}
	return string(code), true
}
