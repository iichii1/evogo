package send_service

import (
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestPollVoteTarget(t *testing.T) {
	own := types.NewADJID("5511000000000", 0, 12)

	tests := []struct {
		name       string
		data       PollVoteStruct
		wantChat   string
		wantSender string
		wantErr    bool
	}{
		{
			name:       "individual chat, poll from the contact",
			data:       PollVoteStruct{Number: "5511999999999", PollMessageID: "ABC"},
			wantChat:   "5511999999999@s.whatsapp.net",
			wantSender: "5511999999999@s.whatsapp.net",
		},
		{
			name:       "poll created by this instance",
			data:       PollVoteStruct{Number: "5511999999999", PollMessageID: "ABC", FromMe: true},
			wantChat:   "5511999999999@s.whatsapp.net",
			wantSender: "5511000000000@s.whatsapp.net",
		},
		{
			name:       "group poll with participant",
			data:       PollVoteStruct{Number: "120363000000000000@g.us", PollMessageID: "ABC", Participant: "5511888888888"},
			wantChat:   "120363000000000000@g.us",
			wantSender: "5511888888888@s.whatsapp.net",
		},
		{
			name:    "group poll without participant",
			data:    PollVoteStruct{Number: "120363000000000000@g.us", PollMessageID: "ABC"},
			wantErr: true,
		},
		{
			name:    "missing poll id",
			data:    PollVoteStruct{Number: "5511999999999"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chat, sender, err := pollVoteTarget(&tt.data, own)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if chat.String() != tt.wantChat {
				t.Fatalf("chat = %s, want %s", chat, tt.wantChat)
			}
			if sender.String() != tt.wantSender {
				t.Fatalf("sender = %s, want %s", sender, tt.wantSender)
			}
		})
	}
}
