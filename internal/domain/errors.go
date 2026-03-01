package domain

import "errors"

var (
	ErrUnsupportedPlatform = errors.New("unsupported platform")
	ErrVideoTooLarge       = errors.New("video is too large")
	ErrDownloadFailed      = errors.New("download failed")
	ErrUnauthorized        = errors.New("user not authorized")
)
