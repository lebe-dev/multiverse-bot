package detector

import (
	"regexp"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

type entry struct {
	platform domain.Platform
	re       *regexp.Regexp
}

type RegexDetector struct {
	patterns []entry
}

func New() *RegexDetector {
	return &RegexDetector{
		patterns: []entry{
			{domain.PlatformYouTube, regexp.MustCompile(`(?i)(?:youtube\.com|youtu\.be)/`)},
			{domain.PlatformInstagram, regexp.MustCompile(`(?i)instagram\.com/(?:p|reel|reels|tv)/`)},
			{domain.PlatformTwitter, regexp.MustCompile(`(?i)(?:twitter\.com|x\.com)/`)},
			{domain.PlatformThreads, regexp.MustCompile(`(?i)threads\.(?:net|com)/`)},
		},
	}
}

func (d *RegexDetector) DisablePlatform(p domain.Platform) {
	filtered := d.patterns[:0]
	for _, e := range d.patterns {
		if e.platform != p {
			filtered = append(filtered, e)
		}
	}
	d.patterns = filtered
}

func (d *RegexDetector) Detect(url string) domain.Platform {
	for _, e := range d.patterns {
		if e.re.MatchString(url) {
			return e.platform
		}
	}
	return domain.PlatformUnknown
}
