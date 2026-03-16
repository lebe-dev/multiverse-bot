package lovethreads

import (
	"testing"
)

func TestParseVideoURLs(t *testing.T) {
	tests := []struct {
		name      string
		html      string
		wantCount int
		wantFirst string
	}{
		{
			name:      "single video",
			html:      `<li><span class="icon-dlvideo"></span><a title="Download Video" href="https://lovethreads.net/dl/video.mp4">Download</a></li>`,
			wantCount: 1,
			wantFirst: "https://lovethreads.net/dl/video.mp4",
		},
		{
			name: "multiple videos",
			html: `<ul class="download-box">
				<li><span class="icon-dlvideo"></span><a title="Download Video" href="https://lovethreads.net/dl/v1.mp4">Download</a></li>
				<li><span class="icon-dlvideo"></span><a title="Download Video" href="https://lovethreads.net/dl/v2.mp4">Download</a></li>
			</ul>`,
			wantCount: 2,
			wantFirst: "https://lovethreads.net/dl/v1.mp4",
		},
		{
			name:      "photo only — no videos",
			html:      `<li><span class="icon-dlimage"></span><select class="photo-option"><option value="https://img.jpg">1080x1350</option></select></li>`,
			wantCount: 0,
		},
		{
			name:      "empty html",
			html:      "",
			wantCount: 0,
		},
		{
			name:      "html-encoded ampersand in URL",
			html:      `<a title="Download Video" href="https://lovethreads.net/dl/video.mp4?a=1&amp;b=2">Download</a>`,
			wantCount: 1,
			wantFirst: "https://lovethreads.net/dl/video.mp4?a=1&b=2",
		},
		{
			name:      "href before title",
			html:      `<a href="https://lovethreads.net/dl/video.mp4" title="Download Video">Download</a>`,
			wantCount: 1,
			wantFirst: "https://lovethreads.net/dl/video.mp4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			urls := parseVideoURLs(tt.html)
			if len(urls) != tt.wantCount {
				t.Fatalf("got %d URLs, want %d", len(urls), tt.wantCount)
			}
			if tt.wantCount > 0 && urls[0] != tt.wantFirst {
				t.Errorf("got URL %q, want %q", urls[0], tt.wantFirst)
			}
		})
	}
}
