package domain

type Platform int

const (
	PlatformUnknown Platform = iota
	PlatformYouTube
	PlatformInstagram
	PlatformTwitter
	PlatformThreads
)

func (p Platform) String() string {
	switch p {
	case PlatformYouTube:
		return "youtube"
	case PlatformInstagram:
		return "instagram"
	case PlatformTwitter:
		return "twitter"
	case PlatformThreads:
		return "threads"
	default:
		return "unknown"
	}
}
