package domain

type PlatformDetector interface {
	Detect(url string) Platform
}
