package autoreply

import (
	"context"
	"fmt"
)

// BotServiceAdapter adapts the bot service to the BotSender interface
type BotServiceAdapter struct {
	sendTextFn func(ctx context.Context, botID uint64, toNodeNum int64, text string) error
}

// NewBotServiceAdapter creates a new bot service adapter
func NewBotServiceAdapter(
	sendTextFn func(ctx context.Context, botID uint64, toNodeNum int64, text string) error,
) *BotServiceAdapter {
	return &BotServiceAdapter{
		sendTextFn: sendTextFn,
	}
}

// SendText sends a text message via the bot service
func (a *BotServiceAdapter) SendText(ctx context.Context, botID uint64, toNodeNum int64, text string) error {
	if a.sendTextFn == nil {
		return fmt.Errorf("send text function is nil")
	}
	return a.sendTextFn(ctx, botID, toNodeNum, text)
}
