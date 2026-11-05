package user_service

import (
	"testing"

	"go.mau.fi/whatsmeow/appstate"
)

// Regressão de #176: o nome do perfil era enviado como nome de grupo com JID
// vazio e a requisição nunca terminava. Agora vai como patch de app state.
func TestBuildProfileNamePatch(t *testing.T) {
	patch, err := buildProfileNamePatch("  Maria Silva  ")
	if err != nil {
		t.Fatal(err)
	}
	if patch.Type != appstate.WAPatchCriticalBlock {
		t.Fatalf("patch type = %v, want %v", patch.Type, appstate.WAPatchCriticalBlock)
	}
	if len(patch.Mutations) != 1 {
		t.Fatalf("mutations = %d, want 1", len(patch.Mutations))
	}
	m := patch.Mutations[0]
	if len(m.Index) != 1 || m.Index[0] != appstate.IndexSettingPushName {
		t.Fatalf("index = %v", m.Index)
	}
	if got := m.Value.GetPushNameSetting().GetName(); got != "Maria Silva" {
		t.Fatalf("name = %q, want it trimmed", got)
	}
}

func TestBuildProfileNamePatchRejectsBlankName(t *testing.T) {
	for _, name := range []string{"", "   ", "\t\n"} {
		if _, err := buildProfileNamePatch(name); err == nil {
			t.Fatalf("%q should be rejected", name)
		}
	}
}
