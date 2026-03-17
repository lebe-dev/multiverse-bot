package domain

import "time"

type Subscription struct {
	ID          int64
	UserID      int64
	ChannelID   string
	ChannelName string
	CreatedAt   time.Time
}

type FeedVideo struct {
	VideoID     string
	ChannelID   string
	ChannelName string
	Title       string
	URL         string
	Published   time.Time
}
