package domain

import (
	"context"
	"time"
)

// DeviceAuthInfo contains info to show a user during device auth flow.
type DeviceAuthInfo struct {
	UserCode        string
	VerificationURI string
	Expiry          time.Time
}

// DriveManager handles per-user Google Drive OAuth and file uploads.
type DriveManager interface {
	// StartAuth initiates the device auth flow. Returns info to show the user
	// and a poll function that blocks until the user approves or the code expires.
	StartAuth(ctx context.Context) (DeviceAuthInfo, func(ctx context.Context, userID int64) error, error)
	IsConnected(userID int64) bool
	Disconnect(userID int64)
	Upload(ctx context.Context, userID int64, title, filePath string) (link string, err error)
}
