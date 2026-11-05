package whatsmeow_service

import (
	"testing"
	"time"

	"github.com/evolution-foundation/evolution-go/pkg/config"
	logger_wrapper "github.com/evolution-foundation/evolution-go/pkg/logger"
	"github.com/evolution-foundation/evolution-go/pkg/runtimestate"
)

func TestNextReconnectDelayBacksOff(t *testing.T) {
	states := runtimestate.New[string, *reconnectState]()
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

	want := []time.Duration{0, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 32 * time.Second, 64 * time.Second, 2 * time.Minute, 2 * time.Minute}
	for i, expected := range want {
		got := nextReconnectDelay(states, "inst", now.Add(time.Duration(i)*time.Second))
		if got != expected {
			t.Fatalf("attempt %d: delay = %s, want %s", i, got, expected)
		}
	}
}

func TestNextReconnectDelayResetsAfterQuietPeriod(t *testing.T) {
	states := runtimestate.New[string, *reconnectState]()
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

	nextReconnectDelay(states, "inst", now)
	nextReconnectDelay(states, "inst", now.Add(time.Second))
	nextReconnectDelay(states, "inst", now.Add(2*time.Second))

	got := nextReconnectDelay(states, "inst", now.Add(2*time.Second+reconnectBackoffReset+time.Second))
	if got != 0 {
		t.Fatalf("delay after quiet period = %s, want 0", got)
	}
}

func TestNextReconnectDelayIsPerInstance(t *testing.T) {
	states := runtimestate.New[string, *reconnectState]()
	now := time.Now()

	nextReconnectDelay(states, "a", now)
	nextReconnectDelay(states, "a", now)

	if got := nextReconnectDelay(states, "b", now); got != 0 {
		t.Fatalf("first reconnect of another instance waited %s", got)
	}
}

func TestSilentStopReasons(t *testing.T) {
	for _, reason := range []string{stopReasonReconnect, stopReasonReplaced, stopReasonLoggedOut} {
		if !isSilentStopReason(reason) {
			t.Fatalf("%q should not emit LoggedOut", reason)
		}
	}
	for _, reason := range []string{stopReasonQRTimeout, stopReasonAPI, stopReasonDelete, "logged out via API"} {
		if isSilentStopReason(reason) {
			t.Fatalf("%q should emit LoggedOut", reason)
		}
	}
}

func newTestService(t *testing.T) *whatsmeowService {
	return &whatsmeowService{
		config:           &config.Config{},
		sessions:         runtimestate.New[string, *runtimestate.Session](),
		reconnectBackoff: runtimestate.New[string, *reconnectState](),
		loggerWrapper:    logger_wrapper.NewLoggerManager(&config.Config{LogDirectory: t.TempDir()}),
	}
}

func TestStopClientWithoutSession(t *testing.T) {
	w := newTestService(t)
	if w.StopClient("missing", stopReasonAPI) {
		t.Fatal("StopClient should report that nothing was running")
	}
}

func TestStopClientWaitsForCleanup(t *testing.T) {
	w := newTestService(t)
	session := runtimestate.NewSession()
	w.sessions.Set("inst", session)

	cleaned := make(chan struct{})
	go func() {
		<-session.Stopping()
		time.Sleep(20 * time.Millisecond)
		close(cleaned)
		session.Finish()
	}()

	if !w.StopClient("inst", stopReasonAPI) {
		t.Fatal("StopClient should report a running session")
	}

	select {
	case <-cleaned:
	default:
		t.Fatal("StopClient returned before cleanup finished")
	}
	if session.Reason() != stopReasonAPI {
		t.Fatalf("reason = %q", session.Reason())
	}
}
