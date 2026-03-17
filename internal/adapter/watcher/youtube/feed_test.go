package youtube

import (
	"strings"
	"testing"
)

const sampleFeedXML = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns:yt="http://www.youtube.com/xml/schemas/2015"
      xmlns:media="http://search.yahoo.com/mrss/"
      xmlns="http://www.w3.org/2005/Atom">
  <link rel="self" href="https://www.youtube.com/feeds/videos.xml?channel_id=UCxxxxxx"/>
  <id>yt:channel:UCxxxxxx</id>
  <yt:channelId>UCxxxxxx</yt:channelId>
  <title>Test Channel</title>
  <entry>
    <id>yt:video:abc12345678</id>
    <yt:videoId>abc12345678</yt:videoId>
    <yt:channelId>UCxxxxxx</yt:channelId>
    <title>Test Video 1</title>
    <link rel="alternate" href="https://www.youtube.com/watch?v=abc12345678"/>
    <author>
      <name>Test Channel</name>
    </author>
    <published>2024-01-15T10:00:00+00:00</published>
  </entry>
  <entry>
    <id>yt:video:def98765432</id>
    <yt:videoId>def98765432</yt:videoId>
    <yt:channelId>UCxxxxxx</yt:channelId>
    <title>Test Video 2</title>
    <link rel="alternate" href="https://www.youtube.com/watch?v=def98765432"/>
    <author>
      <name>Test Channel</name>
    </author>
    <published>2024-01-14T10:00:00+00:00</published>
  </entry>
</feed>`

func TestParseFeed(t *testing.T) {
	videos, err := parseFeed(strings.NewReader(sampleFeedXML))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(videos) != 2 {
		t.Fatalf("expected 2 videos, got %d", len(videos))
	}

	v := videos[0]
	if v.VideoID != "abc12345678" {
		t.Errorf("expected VideoID 'abc12345678', got %q", v.VideoID)
	}
	if v.ChannelID != "UCxxxxxx" {
		t.Errorf("expected ChannelID 'UCxxxxxx', got %q", v.ChannelID)
	}
	if v.ChannelName != "Test Channel" {
		t.Errorf("expected ChannelName 'Test Channel', got %q", v.ChannelName)
	}
	if v.Title != "Test Video 1" {
		t.Errorf("expected Title 'Test Video 1', got %q", v.Title)
	}
	if v.URL != "https://www.youtube.com/watch?v=abc12345678" {
		t.Errorf("unexpected URL: %q", v.URL)
	}
	if v.Published.IsZero() {
		t.Error("expected non-zero Published time")
	}
}

func TestParseFeed_FallbackURL(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns:yt="http://www.youtube.com/xml/schemas/2015" xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <yt:videoId>xyz11111111</yt:videoId>
    <yt:channelId>UCtest</yt:channelId>
    <title>No Link Entry</title>
    <author><name>Channel</name></author>
    <published>2024-01-01T00:00:00+00:00</published>
  </entry>
</feed>`

	videos, err := parseFeed(strings.NewReader(xml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(videos) != 1 {
		t.Fatalf("expected 1 video, got %d", len(videos))
	}
	if videos[0].URL != "https://www.youtube.com/watch?v=xyz11111111" {
		t.Errorf("expected fallback URL, got %q", videos[0].URL)
	}
}

func TestParseFeed_Empty(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Empty Channel</title>
</feed>`

	videos, err := parseFeed(strings.NewReader(xml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(videos) != 0 {
		t.Errorf("expected 0 videos, got %d", len(videos))
	}
}
