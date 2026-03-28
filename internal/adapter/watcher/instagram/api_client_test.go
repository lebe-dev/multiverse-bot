package instagram

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestParseNetscapeCookies(t *testing.T) {
	content := `# Netscape HTTP Cookie File
# This is a generated file! Do not edit.

.instagram.com	TRUE	/	TRUE	0	sessionid	abc123
.instagram.com	TRUE	/	TRUE	0	csrftoken	xyz789
# comment line
.instagram.com	TRUE	/	FALSE	0	ds_user_id	12345
`
	tmp := filepath.Join(t.TempDir(), "cookies.txt")
	if err := os.WriteFile(tmp, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cookies, err := parseNetscapeCookies(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cookies) != 3 {
		t.Fatalf("got %d cookies, want 3", len(cookies))
	}

	want := map[string]string{
		"sessionid":  "abc123",
		"csrftoken":  "xyz789",
		"ds_user_id": "12345",
	}
	for _, c := range cookies {
		if want[c.Name] != c.Value {
			t.Errorf("cookie %s = %q, want %q", c.Name, c.Value, want[c.Name])
		}
	}
}

func TestParseNetscapeCookies_ShortLines(t *testing.T) {
	content := "too\tfew\tfields\n"
	tmp := filepath.Join(t.TempDir(), "cookies.txt")
	if err := os.WriteFile(tmp, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cookies, err := parseNetscapeCookies(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cookies) != 0 {
		t.Fatalf("expected 0 cookies for short lines, got %d", len(cookies))
	}
}

func TestParseUserID(t *testing.T) {
	data := `{"data":{"user":{"id":"67890"}}}`
	id, err := parseUserID([]byte(data))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "67890" {
		t.Errorf("got %q, want %q", id, "67890")
	}
}

func TestParseUserID_Missing(t *testing.T) {
	data := `{"data":{"user":{"id":""}}}`
	_, err := parseUserID([]byte(data))
	if err == nil {
		t.Fatal("expected error for empty user ID")
	}
}

func TestParseReelsMediaJSON(t *testing.T) {
	tests := []struct {
		name         string
		json         string
		wantItems    int
		wantReshare  bool
		wantUsername  string
		wantStoryID  string
	}{
		{
			name: "story with reshare",
			json: `{
				"reels_media": [{
					"items": [{
						"id": "3456789_12345",
						"pk": "3456789",
						"story_reshares": {
							"reshared_story": {
								"user": {"username": "original_author", "pk": 99999},
								"id": "111222_99999"
							}
						}
					}]
				}]
			}`,
			wantItems:   1,
			wantReshare: true,
			wantUsername: "original_author",
			wantStoryID: "111222_99999",
		},
		{
			name: "story without reshare",
			json: `{
				"reels_media": [{
					"items": [{
						"id": "3456789_12345",
						"pk": "3456789"
					}]
				}]
			}`,
			wantItems:   1,
			wantReshare: false,
		},
		{
			name:      "empty reels",
			json:      `{"reels_media": []}`,
			wantItems: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items, err := parseReelsMediaJSON([]byte(tt.json))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(items) != tt.wantItems {
				t.Fatalf("got %d items, want %d", len(items), tt.wantItems)
			}
			if !tt.wantReshare || tt.wantItems == 0 {
				return
			}
			item := items[0]
			if item.StoryReshares == nil || item.StoryReshares.ResharedStory == nil {
				t.Fatal("expected reshare info")
			}
			if item.StoryReshares.ResharedStory.User.Username != tt.wantUsername {
				t.Errorf("reshare username = %q, want %q", item.StoryReshares.ResharedStory.User.Username, tt.wantUsername)
			}
			if item.StoryReshares.ResharedStory.ID != tt.wantStoryID {
				t.Errorf("reshare story ID = %q, want %q", item.StoryReshares.ResharedStory.ID, tt.wantStoryID)
			}
		})
	}
}

func TestMatchStoryID(t *testing.T) {
	tests := []struct {
		name    string
		item    reelsItem
		storyID string
		want    bool
	}{
		{
			name:    "exact ID match",
			item:    reelsItem{ID: "123_456", Pk: json.Number("123")},
			storyID: "123_456",
			want:    true,
		},
		{
			name:    "pk match",
			item:    reelsItem{ID: "123_456", Pk: json.Number("123")},
			storyID: "123",
			want:    true,
		},
		{
			name:    "storyID has pk prefix with underscore",
			item:    reelsItem{ID: "different", Pk: json.Number("123")},
			storyID: "123_789",
			want:    true,
		},
		{
			name:    "item ID is prefix of storyID",
			item:    reelsItem{ID: "123", Pk: json.Number("999")},
			storyID: "123_456",
			want:    true,
		},
		{
			name:    "no match",
			item:    reelsItem{ID: "111", Pk: json.Number("222")},
			storyID: "333",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchStoryID(tt.item, tt.storyID)
			if got != tt.want {
				t.Errorf("matchStoryID(%+v, %q) = %v, want %v", tt.item, tt.storyID, got, tt.want)
			}
		})
	}
}

func TestEnrichStoryMetadata_WithReshare(t *testing.T) {
	cookieFile := writeTempCookies(t)

	profileCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/users/web_profile_info/":
			profileCalls++
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"user": map[string]any{"id": "12345"},
				},
			})
		case "/api/v1/feed/reels_media/":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"reels_media": []any{
					map[string]any{
						"items": []any{
							map[string]any{
								"id": "999_12345",
								"pk": "999",
								"story_reshares": map[string]any{
									"reshared_story": map[string]any{
										"user": map[string]any{"username": "orig_user", "pk": 55555},
										"id":   "888_55555",
									},
								},
							},
						},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := newTestAPIClient(cookieFile, srv.URL)

	reshare, err := client.EnrichStoryMetadata(context.Background(), "testuser", "999")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reshare == nil {
		t.Fatal("expected reshare info, got nil")
	}
	if reshare.Username != "orig_user" {
		t.Errorf("reshare username = %q, want %q", reshare.Username, "orig_user")
	}
	if reshare.StoryID != "888_55555" {
		t.Errorf("reshare story ID = %q, want %q", reshare.StoryID, "888_55555")
	}

	// Call again to verify user ID is cached.
	_, err = client.EnrichStoryMetadata(context.Background(), "testuser", "999")
	if err != nil {
		t.Fatalf("unexpected error on second call: %v", err)
	}
	if profileCalls != 1 {
		t.Errorf("user profile called %d times, want 1 (should be cached)", profileCalls)
	}
}

func TestEnrichStoryMetadata_NoReshare(t *testing.T) {
	cookieFile := writeTempCookies(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/users/web_profile_info/":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"user": map[string]any{"id": "12345"},
				},
			})
		case "/api/v1/feed/reels_media/":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"reels_media": []any{
					map[string]any{
						"items": []any{
							map[string]any{
								"id": "999_12345",
								"pk": "999",
							},
						},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := newTestAPIClient(cookieFile, srv.URL)

	reshare, err := client.EnrichStoryMetadata(context.Background(), "testuser", "999")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reshare != nil {
		t.Errorf("expected nil reshare, got %+v", reshare)
	}
}

func TestEnrichStoryMetadata_APIError(t *testing.T) {
	cookieFile := writeTempCookies(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := newTestAPIClient(cookieFile, srv.URL)

	_, err := client.EnrichStoryMetadata(context.Background(), "testuser", "999")
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}

// --- helpers ---

func writeTempCookies(t *testing.T) string {
	t.Helper()
	content := ".instagram.com\tTRUE\t/\tTRUE\t0\tsessionid\tabc123\n"
	path := filepath.Join(t.TempDir(), "cookies.txt")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func newTestAPIClient(cookieFile, baseURL string) *APIClient {
	c := NewAPIClient(func() string { return cookieFile }, slog.Default())
	// Override the API URL to point to the test server.
	c.apiURL = baseURL
	return c
}
