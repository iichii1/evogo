package whatsmeow_service

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/types/events"
)

func TestOnlyAHashMismatchOfAnIncrementalSyncAsksForAFullOne(t *testing.T) {
	mismatch := fmt.Errorf("failed to verify patch v28: %w", appstate.ErrMismatchingLTHash)
	cases := []struct {
		name string
		evt  *events.AppStateSyncError
		want bool
	}{
		{"hash diferente num patch", &events.AppStateSyncError{Name: appstate.WAPatchRegularHigh, Error: mismatch}, true},
		{"a própria sincronização inteira não repete", &events.AppStateSyncError{Name: appstate.WAPatchRegularHigh, FullSync: true, Error: mismatch}, false},
		{"outro erro", &events.AppStateSyncError{Name: appstate.WAPatchRegularHigh, Error: errors.New("timeout")}, false},
		{"MAC do patch", &events.AppStateSyncError{Name: appstate.WAPatchRegularHigh, Error: appstate.ErrMismatchingPatchMAC}, false},
		{"sem erro", &events.AppStateSyncError{Name: appstate.WAPatchRegularHigh}, false},
		{"evento nulo", nil, false},
	}
	for _, c := range cases {
		if got := shouldResyncAppState(c.evt); got != c.want {
			t.Errorf("%s: esperado %v, veio %v", c.name, c.want, got)
		}
	}
}

func TestAResyncIsAskedOncePerCollectionAndInterval(t *testing.T) {
	var r appStateResync
	start := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	if !r.allow(appstate.WAPatchRegularHigh, start) {
		t.Fatal("a primeira deveria passar")
	}
	if r.allow(appstate.WAPatchRegularHigh, start.Add(appStateResyncCooldown-time.Second)) {
		t.Error("dentro do intervalo não deveria repetir")
	}
	if !r.allow(appstate.WAPatchRegularLow, start.Add(time.Second)) {
		t.Error("outra coleção tem o seu próprio intervalo")
	}
	if !r.allow(appstate.WAPatchRegularHigh, start.Add(appStateResyncCooldown)) {
		t.Error("passado o intervalo, volta a valer")
	}
}
