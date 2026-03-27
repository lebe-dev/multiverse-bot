package domain

import "errors"

var (
	ErrUnsupportedPlatform = errors.New("unsupported platform")
	ErrVideoTooLarge       = errors.New("video is too large")
	ErrDownloadFailed      = errors.New("download failed")
	ErrUnauthorized        = errors.New("user not authorized")
	ErrNotImplemented      = errors.New("not implemented")

	ErrAlreadySubscribed = errors.New("already subscribed")
	ErrNotSubscribed     = errors.New("not subscribed")
	ErrMaxSubscriptions  = errors.New("subscription limit reached")
	ErrChannelNotFound   = errors.New("channel not found")
	ErrMaxChannels       = errors.New("global channel limit reached")

	ErrAlreadySubscribedStory = errors.New("already subscribed to stories")
	ErrNotSubscribedStory     = errors.New("not subscribed to stories")
	ErrUsernameNotFound       = errors.New("instagram username not found")

	ErrAlreadySubscribedPost = errors.New("already subscribed to posts")
	ErrNotSubscribedPost     = errors.New("not subscribed to posts")

	ErrPluginUnavailable = errors.New("plugin unavailable")
	ErrPluginTimeout     = errors.New("plugin timed out")
)
