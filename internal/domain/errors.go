package domain

import "errors"

var (
	ErrUnsupportedPlatform = errors.New("unsupported platform")
	ErrVideoTooLarge       = errors.New("video is too large")
	ErrDownloadFailed      = errors.New("download failed")
	ErrUnauthorized        = errors.New("user not authorized")

	ErrAlreadySubscribed = errors.New("already subscribed")
	ErrNotSubscribed     = errors.New("not subscribed")
	ErrMaxSubscriptions  = errors.New("subscription limit reached")
	ErrChannelNotFound   = errors.New("channel not found")
	ErrMaxChannels       = errors.New("global channel limit reached")
)
