package domain

// MediaType describes the kind of a media item.
type MediaType int

const (
	MediaVideo MediaType = iota
	MediaPhoto
)

// MediaItem is a single downloaded media file (photo or video).
type MediaItem struct {
	Type     MediaType
	FilePath string
	Size     int64
	URL      string // source URL of this specific item
}

// MediaResult is the result of downloading a URL that may contain multiple media items.
type MediaResult struct {
	Items    []MediaItem
	Title    string
	Platform Platform
	URL      string // original post URL
}

// MaxItemSize returns the size of the largest single item.
func (r *MediaResult) MaxItemSize() int64 {
	var max int64
	for _, item := range r.Items {
		if item.Size > max {
			max = item.Size
		}
	}
	return max
}

// TotalSize returns the sum of all item sizes.
func (r *MediaResult) TotalSize() int64 {
	var total int64
	for _, item := range r.Items {
		total += item.Size
	}
	return total
}

// HasVideo returns true if at least one item is a video.
func (r *MediaResult) HasVideo() bool {
	for _, item := range r.Items {
		if item.Type == MediaVideo {
			return true
		}
	}
	return false
}

// AsVideo converts a single-item MediaResult back to *Video for backward compat.
func (r *MediaResult) AsVideo() *Video {
	if len(r.Items) == 0 {
		return nil
	}
	return &Video{
		URL:      r.URL,
		FilePath: r.Items[0].FilePath,
		Platform: r.Platform,
		Size:     r.Items[0].Size,
		Title:    r.Title,
	}
}
