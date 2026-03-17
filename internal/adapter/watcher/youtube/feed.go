package youtube

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"time"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

const feedURLTemplate = "https://www.youtube.com/feeds/videos.xml?channel_id=%s"

type atomFeed struct {
	Title   string      `xml:"title"`
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	VideoID   string     `xml:"http://www.youtube.com/xml/schemas/2015 videoId"`
	ChannelID string     `xml:"http://www.youtube.com/xml/schemas/2015 channelId"`
	Title     string     `xml:"title"`
	Links     []atomLink `xml:"link"`
	Author    atomAuthor `xml:"author"`
	Published string     `xml:"published"`
}

type atomLink struct {
	Rel  string `xml:"rel,attr"`
	Href string `xml:"href,attr"`
}

type atomAuthor struct {
	Name string `xml:"name"`
}

type FeedFetcher struct {
	client *http.Client
}

func NewFeedFetcher() *FeedFetcher {
	return &FeedFetcher{
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (f *FeedFetcher) FetchFeed(ctx context.Context, channelID string) ([]domain.FeedVideo, error) {
	url := fmt.Sprintf(feedURLTemplate, channelID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	return parseFeed(resp.Body)
}

func parseFeed(r io.Reader) ([]domain.FeedVideo, error) {
	var feed atomFeed
	if err := xml.NewDecoder(r).Decode(&feed); err != nil {
		return nil, fmt.Errorf("parsing feed: %w", err)
	}

	videos := make([]domain.FeedVideo, 0, len(feed.Entries))
	for _, e := range feed.Entries {
		pub, _ := time.Parse(time.RFC3339, e.Published)

		href := "https://www.youtube.com/watch?v=" + e.VideoID
		for _, l := range e.Links {
			if l.Rel == "alternate" && l.Href != "" {
				href = l.Href
				break
			}
		}

		videos = append(videos, domain.FeedVideo{
			VideoID:     e.VideoID,
			ChannelID:   e.ChannelID,
			ChannelName: e.Author.Name,
			Title:       e.Title,
			URL:         href,
			Published:   pub,
		})
	}
	return videos, nil
}
