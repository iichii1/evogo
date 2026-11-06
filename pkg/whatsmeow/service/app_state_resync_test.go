package whatsmeow_service

import (
	"context"
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

func TestThePhoneIsAskedForACopyOnlyWhenTheFullDownloadFailsTheSameWay(t *testing.T) {
	mismatch := fmt.Errorf("failed to decode app state regular_high patches: failed to verify patch v28: %w", appstate.ErrMismatchingLTHash)
	start := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name    string
		fetch   error
		ask     error
		want    string
		wantAsk int
	}{
		{"baixou de novo e resolveu", nil, nil, "resynced", 0},
		{"mesmo hash que não bate", mismatch, nil, "recovery-asked", 1},
		{"outro erro não pede a cópia", errors.New("timeout"), nil, "failed", 0},
		{"MAC do patch não pede a cópia", appstate.ErrMismatchingPatchMAC, nil, "failed", 0},
		{"pedido que não saiu", mismatch, errors.New("sem conexão"), "recovery-not-sent", 1},
	}
	for _, c := range cases {
		var r appStateResync
		asked := 0
		outcome, _ := r.resyncOrRecover(context.Background(), appstate.WAPatchRegularHigh, start,
			func(context.Context, appstate.WAPatchName) error { return c.fetch },
			func(context.Context, appstate.WAPatchName) error { asked++; return c.ask })
		if outcome != c.want || asked != c.wantAsk {
			t.Errorf("%s: esperado %s com %d pedido(s), veio %s com %d", c.name, c.want, c.wantAsk, outcome, asked)
		}
	}
}

func TestTheCopyIsAskedOncePerCollectionAndInterval(t *testing.T) {
	mismatch := fmt.Errorf("failed to verify patch v28: %w", appstate.ErrMismatchingLTHash)
	var r appStateResync
	asked := 0
	run := func(name appstate.WAPatchName, at time.Time) string {
		outcome, _ := r.resyncOrRecover(context.Background(), name, at,
			func(context.Context, appstate.WAPatchName) error { return mismatch },
			func(context.Context, appstate.WAPatchName) error { asked++; return nil })
		return outcome
	}
	start := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	if got := run(appstate.WAPatchRegularHigh, start); got != "recovery-asked" {
		t.Fatalf("o primeiro pedido deveria sair, veio %s", got)
	}
	if got := run(appstate.WAPatchRegularHigh, start.Add(appStateRecoveryCooldown-time.Second)); got != "recovery-recently-asked" {
		t.Errorf("dentro do intervalo não deveria repetir, veio %s", got)
	}
	if got := run(appstate.WAPatchRegularLow, start.Add(time.Second)); got != "recovery-asked" {
		t.Errorf("outra coleção tem o seu próprio intervalo, veio %s", got)
	}
	if got := run(appstate.WAPatchRegularHigh, start.Add(appStateRecoveryCooldown)); got != "recovery-asked" {
		t.Errorf("passado o intervalo, volta a valer, veio %s", got)
	}
	if asked != 3 {
		t.Errorf("esperados 3 pedidos ao celular, saíram %d", asked)
	}
}
