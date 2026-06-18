package autoreply

import (
	"context"
	"fmt"
)

// BotServiceAdapter adapts the bot service to the BotSender interface
type BotServiceAdapter struct {
	sendDirectTextFn  func(ctx context.Context, botID uint64, toNodeNum int64, text string) error
	sendChannelTextFn func(ctx context.Context, botID uint64, channelID string, text string) error
}

// NewBotServiceAdapter creates a new bot service adapter
func NewBotServiceAdapter(
	sendDirectTextFn func(ctx context.Context, botID uint64, toNodeNum int64, text string) error,
	sendChannelTextFn func(ctx context.Context, botID uint64, channelID string, text string) error,
) *BotServiceAdapter {
	return &BotServiceAdapter{
		sendDirectTextFn:  sendDirectTextFn,
		sendChannelTextFn: sendChannelTextFn,
	}
}

// SendDirectText sends a direct/private message to a specific node
func (a *BotServiceAdapter) SendDirectText(ctx context.Context, botID uint64, toNodeNum int64, text string) error {
	if a.sendDirectTextFn == nil {
		return fmt.Errorf("send direct text function is nil")
	}
	return a.sendDirectTextFn(ctx, botID, toNodeNum, text)
}

// SendChannelText sends a channel message to a specific channel
func (a *BotServiceAdapter) SendChannelText(ctx context.Context, botID uint64, channelID string, text string) error {
	if a.sendChannelTextFn == nil {
		return fmt.Errorf("send channel text function is nil")
	}
	return a.sendChannelTextFn(ctx, botID, channelID, text)
}
