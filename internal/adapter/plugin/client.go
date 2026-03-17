package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

// httpClient implements domain.PluginClient over HTTP.
type httpClient struct {
	name    string
	baseURL string
	timeout time.Duration
	http    *http.Client
}

func newHTTPClient(name, baseURL string, timeout time.Duration) *httpClient {
	return &httpClient{
		name:    name,
		baseURL: baseURL,
		timeout: timeout,
		http:    &http.Client{Timeout: timeout},
	}
}

// --- JSON wire types (lowercase snake_case for the plugin API) ---

type manifestResponse struct {
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Commands    []manifestCommand  `json:"commands"`
	URLPatterns []string           `json:"url_patterns"`
}

type manifestCommand struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}

type executeRequest struct {
	Trigger   triggerJSON `json:"trigger"`
	User      userJSON    `json:"user"`
	MessageID int         `json:"message_id"`
}

type triggerJSON struct {
	Type           string `json:"type"`
	Command        string `json:"command,omitempty"`
	Args           string `json:"args,omitempty"`
	URL            string `json:"url,omitempty"`
	MatchedPattern string `json:"matched_pattern,omitempty"`
	RawText        string `json:"raw_text"`
}

type userJSON struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

type executeResponse struct {
	Actions []actionJSON `json:"actions"`
	Toast   string       `json:"toast,omitempty"`
	Error   string       `json:"error,omitempty"`
}

type actionJSON struct {
	Type      string       `json:"type"`
	Text      string       `json:"text,omitempty"`
	ParseMode string       `json:"parse_mode,omitempty"`
	Buttons   []buttonJSON `json:"buttons,omitempty"`
	URL       string       `json:"url,omitempty"`
	Filename  string       `json:"filename,omitempty"`
	Caption   string       `json:"caption,omitempty"`
	MimeType  string       `json:"mime_type,omitempty"`
	MessageID int          `json:"message_id,omitempty"`
}

type buttonJSON struct {
	Text     string `json:"text"`
	URL      string `json:"url,omitempty"`
	Callback string `json:"callback,omitempty"`
}

type callbackRequest struct {
	CallbackID string   `json:"callback_id"`
	User       userJSON `json:"user"`
	MessageID  int      `json:"message_id"`
}

func (c *httpClient) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return fmt.Errorf("%w: %v", domain.ErrPluginUnavailable, err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", domain.ErrPluginUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: health returned %d", domain.ErrPluginUnavailable, resp.StatusCode)
	}
	return nil
}

func (c *httpClient) Manifest(ctx context.Context) (*domain.PluginManifest, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/manifest", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrPluginUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("manifest returned %d", resp.StatusCode)
	}

	var mr manifestResponse
	if err := json.NewDecoder(resp.Body).Decode(&mr); err != nil {
		return nil, fmt.Errorf("decoding manifest: %w", err)
	}

	m := &domain.PluginManifest{
		Name:        mr.Name,
		Description: mr.Description,
		URLPatterns: mr.URLPatterns,
	}
	for _, cmd := range mr.Commands {
		m.Commands = append(m.Commands, domain.PluginCommand{
			Command:     cmd.Command,
			Description: cmd.Description,
		})
	}
	return m, nil
}

func (c *httpClient) Execute(ctx context.Context, dreq *domain.PluginRequest) (*domain.PluginResponse, error) {
	body := executeRequest{
		Trigger: triggerJSON{
			Type:           dreq.Trigger.Type,
			Command:        dreq.Trigger.Command,
			Args:           dreq.Trigger.Args,
			URL:            dreq.Trigger.URL,
			MatchedPattern: dreq.Trigger.MatchedPattern,
			RawText:        dreq.Trigger.RawText,
		},
		User: userJSON{
			ID:       dreq.User.ID,
			Username: dreq.User.Username,
		},
		MessageID: dreq.MessageID,
	}

	return c.postJSON(ctx, "/execute", body)
}

func (c *httpClient) Callback(ctx context.Context, dreq *domain.PluginCallbackRequest) (*domain.PluginResponse, error) {
	body := callbackRequest{
		CallbackID: dreq.CallbackID,
		User: userJSON{
			ID:       dreq.User.ID,
			Username: dreq.User.Username,
		},
		MessageID: dreq.MessageID,
	}

	return c.postJSON(ctx, "/callback", body)
}

func (c *httpClient) postJSON(ctx context.Context, path string, payload any) (*domain.PluginResponse, error) {
	buf, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrPluginTimeout, err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MB limit
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var er executeResponse
	if err := json.Unmarshal(respBody, &er); err != nil {
		if resp.StatusCode >= 500 {
			return nil, fmt.Errorf("%w: HTTP %d", domain.ErrPluginUnavailable, resp.StatusCode)
		}
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if er.Error != "" {
		return &domain.PluginResponse{Error: er.Error}, nil
	}

	pr := &domain.PluginResponse{
		Toast: er.Toast,
	}
	for _, a := range er.Actions {
		pa := domain.PluginAction{
			Type:      a.Type,
			Text:      a.Text,
			ParseMode: a.ParseMode,
			URL:       a.URL,
			Filename:  a.Filename,
			Caption:   a.Caption,
			MimeType:  a.MimeType,
			MessageID: a.MessageID,
		}
		for _, btn := range a.Buttons {
			pa.Buttons = append(pa.Buttons, domain.PluginButton{
				Text:     btn.Text,
				URL:      btn.URL,
				Callback: btn.Callback,
			})
		}
		pr.Actions = append(pr.Actions, pa)
	}

	return pr, nil
}
