package message_service

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

type fakePresenceSender struct {
	mu    sync.Mutex
	sends []types.Presence
}

func (f *fakePresenceSender) SendPresence(_ context.Context, state types.Presence) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sends = append(f.sends, state)
	return nil
}

func (f *fakePresenceSender) snapshot() []types.Presence {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]types.Presence(nil), f.sends...)
}

func TestPresenceRestorerSendsUnavailableAfterDelay(t *testing.T) {
	restorer := newPresenceRestorer(20*time.Millisecond, nil)
	sender := &fakePresenceSender{}

	restorer.schedule("inst", sender)

	time.Sleep(80 * time.Millisecond)
	got := sender.snapshot()
	if len(got) != 1 || got[0] != types.PresenceUnavailable {
		t.Fatalf("sends = %v, want one unavailable", got)
	}
}

func TestPresenceRestorerDebouncesRepeatedCalls(t *testing.T) {
	restorer := newPresenceRestorer(40*time.Millisecond, nil)
	sender := &fakePresenceSender{}

	for i := 0; i < 5; i++ {
		restorer.schedule("inst", sender)
		time.Sleep(10 * time.Millisecond)
	}

	if got := sender.snapshot(); len(got) != 0 {
		t.Fatalf("restored too early: %v", got)
	}

	time.Sleep(100 * time.Millisecond)
	if got := sender.snapshot(); len(got) != 1 {
		t.Fatalf("sends = %v, want exactly one", got)
	}
}

func TestPresenceRestorerKeepsInstancesSeparate(t *testing.T) {
	restorer := newPresenceRestorer(20*time.Millisecond, nil)
	a := &fakePresenceSender{}
	b := &fakePresenceSender{}

	restorer.schedule("a", a)
	restorer.schedule("b", b)

	time.Sleep(80 * time.Millisecond)
	if len(a.snapshot()) != 1 || len(b.snapshot()) != 1 {
		t.Fatalf("a=%v b=%v, want one send each", a.snapshot(), b.snapshot())
	}
}
