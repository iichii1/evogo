package send_service

import (
	"encoding/json"
	"testing"
)

func decodeParams(t *testing.T, raw string) map[string]string {
	t.Helper()
	var out map[string]string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("params are not valid JSON (%v): %s", err, raw)
	}
	return out
}

func TestBuildCarouselButton(t *testing.T) {
	tests := []struct {
		name       string
		btn        CarouselButtonStruct
		wantName   string
		wantParams map[string]string
	}{
		{
			name:       "url field (issue #51 payload)",
			btn:        CarouselButtonStruct{Type: "URL", DisplayText: "Visitar Tienda 🛒", URL: "https://RESTAURANT.com/?a=1&b=2"},
			wantName:   "cta_url",
			wantParams: map[string]string{"display_text": "Visitar Tienda 🛒", "url": "https://RESTAURANT.com/?a=1&b=2"},
		},
		{
			name:       "url in id still works",
			btn:        CarouselButtonStruct{Type: "url", DisplayText: "Site", Id: "https://example.com"},
			wantName:   "cta_url",
			wantParams: map[string]string{"display_text": "Site", "url": "https://example.com"},
		},
		{
			name:       "phoneNumber field",
			btn:        CarouselButtonStruct{Type: "CALL", DisplayText: "Llamar a Soporte 📞", PhoneNumber: "+57313623432"},
			wantName:   "cta_call",
			wantParams: map[string]string{"display_text": "Llamar a Soporte 📞", "phone_number": "+57313623432"},
		},
		{
			name:       "COPY_CODE alias",
			btn:        CarouselButtonStruct{Type: "COPY_CODE", DisplayText: "Copiar", CopyCode: "RESTAURANT2026"},
			wantName:   "cta_copy",
			wantParams: map[string]string{"display_text": "Copiar", "copy_code": "RESTAURANT2026"},
		},
		{
			name:       "reply is the default",
			btn:        CarouselButtonStruct{DisplayText: "Quero", Id: "card1"},
			wantName:   "quick_reply",
			wantParams: map[string]string{"display_text": "Quero", "id": "card1"},
		},
		{
			name:       "quotes and backslashes are escaped",
			btn:        CarouselButtonStruct{Type: "REPLY", DisplayText: `Diga "oi" \ agora`, Id: `a"b`},
			wantName:   "quick_reply",
			wantParams: map[string]string{"display_text": `Diga "oi" \ agora`, "id": `a"b`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, raw, err := buildCarouselButton(tt.btn)
			if err != nil {
				t.Fatal(err)
			}
			if name != tt.wantName {
				t.Fatalf("name = %q, want %q", name, tt.wantName)
			}
			got := decodeParams(t, raw)
			for k, v := range tt.wantParams {
				if got[k] != v {
					t.Fatalf("%s = %q, want %q (raw %s)", k, got[k], v, raw)
				}
			}
			if len(got) != len(tt.wantParams) {
				t.Fatalf("unexpected params: %s", raw)
			}
		})
	}
}

func TestBuildCarouselButtonRejectsIncompleteButtons(t *testing.T) {
	bad := []CarouselButtonStruct{
		{Type: "URL", DisplayText: "Site"},
		{Type: "CALL", DisplayText: "Ligar"},
		{Type: "COPY", DisplayText: "Copiar"},
		{Type: "PIX", DisplayText: "Pagar"},
	}
	for _, btn := range bad {
		if _, _, err := buildCarouselButton(btn); err == nil {
			t.Fatalf("expected an error for %+v", btn)
		}
	}
}
