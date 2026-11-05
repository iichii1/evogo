package whatsmeow_service

import (
	"testing"

	"go.mau.fi/whatsmeow/store"
)

func TestResolveWAVersion(t *testing.T) {
	current := store.WAVersionContainer{2, 3000, 1052556849}

	tests := []struct {
		name       string
		configured *clientVersion
		fetched    *clientVersion
		want       store.WAVersionContainer
	}{
		{
			name: "nothing available keeps current",
			want: current,
		},
		{
			name:    "newer fetched version wins",
			fetched: &clientVersion{2, 3000, 1052600000},
			want:    store.WAVersionContainer{2, 3000, 1052600000},
		},
		{
			name:    "older fetched version does not downgrade",
			fetched: &clientVersion{2, 3000, 1042386815},
			want:    current,
		},
		{
			name:    "invalid fetched version is ignored",
			fetched: &clientVersion{2, 3000, 0},
			want:    current,
		},
		{
			name:       "configured version is used even if older",
			configured: &clientVersion{2, 3000, 1042386815},
			fetched:    &clientVersion{2, 3000, 1052600000},
			want:       store.WAVersionContainer{2, 3000, 1042386815},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveWAVersion(tt.configured, tt.fetched, current)
			if got != tt.want {
				t.Fatalf("resolveWAVersion() = %s, want %s", got.String(), tt.want.String())
			}
		})
	}
}

func TestApplyWAVersionUpdatesHandshakePayload(t *testing.T) {
	original := store.GetWAVersion()
	t.Cleanup(func() { store.SetWAVersion(original) })

	next := clientVersion{
		Major: int(original[0]),
		Minor: int(original[1]),
		Patch: int(original[2]) + 1,
	}

	previous, applied := applyWAVersion(nil, &next)
	if previous != original {
		t.Fatalf("previous = %s, want %s", previous.String(), original.String())
	}
	if applied != next.toContainer() {
		t.Fatalf("applied = %s, want %s", applied.String(), next.toContainer().String())
	}
	if store.GetWAVersion() != next.toContainer() {
		t.Fatalf("store.GetWAVersion() = %s, want %s", store.GetWAVersion().String(), next.toContainer().String())
	}

	// O ClientPayload do handshake é o que o servidor confere para decidir
	// se responde 405; ele precisa acompanhar a nova versão.
	appVersion := store.BaseClientPayload.GetUserAgent().GetAppVersion()
	if appVersion.GetTertiary() != uint32(next.Patch) {
		t.Fatalf("handshake AppVersion tertiary = %d, want %d", appVersion.GetTertiary(), next.Patch)
	}
	if store.DeviceProps.GetVersion().GetTertiary() != uint32(next.Patch) {
		t.Fatalf("DeviceProps tertiary = %d, want %d", store.DeviceProps.GetVersion().GetTertiary(), next.Patch)
	}
}
