package telegram

import (
	"errors"
	"testing"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

func TestWatchSubscribeError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		contains string
	}{
		{"already subscribed", domain.ErrAlreadySubscribed, "уже подписаны"},
		{"max subscriptions", domain.ErrMaxSubscriptions, "лимит подписок"},
		{"max channels", domain.ErrMaxChannels, "глобальный лимит"},
		{"channel not found", domain.ErrChannelNotFound, "Канал не найден"},
		{"unknown error", errors.New("something broke"), "Не удалось подписаться"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := watchSubscribeError(tt.err)
			if !containsStr(got, tt.contains) {
				t.Errorf("watchSubscribeError(%v) = %q, want it to contain %q", tt.err, got, tt.contains)
			}
		})
	}
}

func TestFriendlyDownloadError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		contains string
	}{
		{"video too large", domain.ErrVideoTooLarge, "слишком большое"},
		{"unsupported platform", domain.ErrUnsupportedPlatform, "неподдерживаемая"},
		{"generic error", errors.New("timeout"), "попробуйте позже"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := friendlyDownloadError(tt.err)
			if !containsStr(got, tt.contains) {
				t.Errorf("friendlyDownloadError(%v) = %q, want it to contain %q", tt.err, got, tt.contains)
			}
		})
	}
}

func TestWatchListMessage(t *testing.T) {
	subs := []domain.Subscription{
		{ChannelID: "UC123", ChannelName: "Channel One"},
		{ChannelID: "UC456", ChannelName: "Channel Two"},
	}

	text, kb := watchListMessage(subs)

	if !containsStr(text, "(2)") {
		t.Errorf("expected count in text, got %q", text)
	}
	if len(kb.InlineKeyboard) != 2 {
		t.Errorf("expected 2 keyboard rows, got %d", len(kb.InlineKeyboard))
	}
	if kb.InlineKeyboard[0][0].Data != "watch_rm:UC123" {
		t.Errorf("expected watch_rm:UC123, got %q", kb.InlineKeyboard[0][0].Data)
	}
	if kb.InlineKeyboard[1][0].Data != "watch_rm:UC456" {
		t.Errorf("expected watch_rm:UC456, got %q", kb.InlineKeyboard[1][0].Data)
	}
}
