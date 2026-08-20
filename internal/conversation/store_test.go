package conversation

import (
	"testing"

	"meshtastic_mqtt_server/internal/message"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	return NewStore(t.TempDir())
}

func TestGetOrCreateForBotPeerIsolation(t *testing.T) {
	s := newTestStore(t)

	convA, err := s.GetOrCreateForBot(1, "bot1", "nodeA")
	if err != nil {
		t.Fatal(err)
	}
	convB, err := s.GetOrCreateForBot(1, "bot1", "nodeB")
	if err != nil {
		t.Fatal(err)
	}
	if convA.ID == convB.ID {
		t.Fatal("different peers must get different conversations")
	}
	if convA.PeerNodeID != "nodeA" || convB.PeerNodeID != "nodeB" {
		t.Fatalf("peer ids not set: %q %q", convA.PeerNodeID, convB.PeerNodeID)
	}

	// 再次获取应命中同一会话(内容互不可见)。
	convA2, err := s.GetOrCreateForBot(1, "bot1", "nodeA")
	if err != nil {
		t.Fatal(err)
	}
	if convA2.ID != convA.ID {
		t.Fatalf("peer A should reuse its conversation, got %s want %s", convA2.ID, convA.ID)
	}

	// 不同 bot 的会话互不影响。
	convOtherBot, err := s.GetOrCreateForBot(2, "bot2", "nodeA")
	if err != nil {
		t.Fatal(err)
	}
	if convOtherBot.ID == convA.ID {
		t.Fatal("different bots must get different conversations")
	}
}

func TestGetOrCreateForBotLegacyNoPeer(t *testing.T) {
	s := newTestStore(t)
	c1, err := s.GetOrCreateForBot(1, "bot1", "")
	if err != nil {
		t.Fatal(err)
	}
	c2, err := s.GetOrCreateForBot(1, "bot1", "")
	if err != nil {
		t.Fatal(err)
	}
	if c1.ID != c2.ID {
		t.Fatal("no-peer callers should share the most recent conversation")
	}
}

func TestAddMessageHistoryCap(t *testing.T) {
	s := newTestStore(t)
	conv, err := s.GetOrCreateForBot(1, "bot1", "nodeA")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 120; i++ {
		if err := s.AddMessage(conv.ID, message.ChatMessage{Role: "user", Content: "m"}); err != nil {
			t.Fatal(err)
		}
	}
	conv, err = s.Get(conv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(conv.Messages) != maxConversationMessages {
		t.Fatalf("history must be capped at %d, got %d", maxConversationMessages, len(conv.Messages))
	}
	if conv.Messages[0].Content != "m" || conv.Messages[len(conv.Messages)-1].Content != "m" {
		t.Fatal("messages content intact")
	}
}
