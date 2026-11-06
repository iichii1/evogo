package message_service

import (
	"bytes"
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waMmsRetry"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"go.mau.fi/whatsmeow/util/gcmutil"
	"go.mau.fi/whatsmeow/util/hkdfutil"
	"google.golang.org/protobuf/proto"
)

var testMediaKey = bytes.Repeat([]byte{7}, 32)

// retryAnswer cifra a resposta do celular como o whatsmeow espera recebê-la.
func retryAnswer(t *testing.T, messageID string, result waMmsRetry.MediaRetryNotification_ResultType, path string) *events.MediaRetry {
	t.Helper()
	plain, err := proto.Marshal(&waMmsRetry.MediaRetryNotification{
		StanzaID: proto.String(messageID), DirectPath: proto.String(path), Result: result.Enum(),
	})
	if err != nil {
		t.Fatal(err)
	}
	key := hkdfutil.SHA256(testMediaKey, nil, []byte("WhatsApp Media Retry Notification"), 32)
	iv := bytes.Repeat([]byte{1}, 12)
	ciphertext, err := gcmutil.Encrypt(key, iv, plain, []byte(messageID))
	if err != nil {
		t.Fatal(err)
	}
	return &events.MediaRetry{MessageID: messageID, Ciphertext: ciphertext, IV: iv}
}

type retryFixture struct {
	sent      atomic.Int32
	downloads atomic.Int32
	answer    func() *events.MediaRetry
	sendErr   error
}

func (f *retryFixture) retrier(timeout time.Duration) mediaRetrier {
	response := make(chan *events.MediaRetry, 1)
	return mediaRetrier{
		send: func(_ context.Context, info *types.MessageInfo, key []byte) error {
			f.sent.Add(1)
			if f.sendErr != nil {
				return f.sendErr
			}
			if !bytes.Equal(key, testMediaKey) {
				return errors.New("chave de mídia errada")
			}
			if f.answer != nil {
				response <- f.answer()
			}
			return nil
		},
		wait: func(string) (<-chan *events.MediaRetry, func()) { return response, func() {} },
		download: func(_ context.Context, media whatsmeow.DownloadableMessage) ([]byte, error) {
			f.downloads.Add(1)
			if media.GetDirectPath() == "/novo" {
				return []byte("arquivo"), nil
			}
			return nil, whatsmeow.ErrMediaDownloadFailedWith410
		},
		timeout: timeout,
	}
}

func expiredImage() *waE2E.ImageMessage {
	return &waE2E.ImageMessage{DirectPath: proto.String("/velho"), MediaKey: testMediaKey}
}

var retryInfo = &types.MessageInfo{ID: "ABC"}

func TestDownloadAsksThePhoneWhenTheMediaExpired(t *testing.T) {
	f := &retryFixture{}
	f.answer = func() *events.MediaRetry {
		return retryAnswer(t, "ABC", waMmsRetry.MediaRetryNotification_SUCCESS, "/novo")
	}
	data, err := f.retrier(time.Second).fetch(context.Background(), expiredImage(), retryInfo)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "arquivo" {
		t.Fatalf("baixou %q", data)
	}
	if f.sent.Load() != 1 || f.downloads.Load() != 2 {
		t.Fatalf("pedidos=%d downloads=%d, esperava 1 e 2", f.sent.Load(), f.downloads.Load())
	}
}

func TestDownloadWithoutInfoKeepsTheOriginalError(t *testing.T) {
	f := &retryFixture{}
	_, err := f.retrier(time.Second).fetch(context.Background(), expiredImage(), nil)
	if !errors.Is(err, whatsmeow.ErrMediaDownloadFailedWith410) {
		t.Fatalf("erro %v", err)
	}
	if f.sent.Load() != 0 {
		t.Fatal("sem info não pode pedir ao celular")
	}
}

func TestDownloadOtherErrorsDoNotAskThePhone(t *testing.T) {
	f := &retryFixture{}
	r := f.retrier(time.Second)
	want := errors.New("rede fora")
	r.download = func(context.Context, whatsmeow.DownloadableMessage) ([]byte, error) { return nil, want }
	if _, err := r.fetch(context.Background(), expiredImage(), retryInfo); !errors.Is(err, want) {
		t.Fatalf("erro %v", err)
	}
	if f.sent.Load() != 0 {
		t.Fatal("só 404 e 410 pedem ao celular")
	}
}

func TestDownloadReportsThePhoneNoLongerHasTheMedia(t *testing.T) {
	cases := map[string]func() *events.MediaRetry{
		"resultado NOT_FOUND": func() *events.MediaRetry {
			return retryAnswer(t, "ABC", waMmsRetry.MediaRetryNotification_NOT_FOUND, "")
		},
		"erro 2 sem cifra": func() *events.MediaRetry {
			return &events.MediaRetry{MessageID: "ABC", Error: &events.MediaRetryError{Code: 2}}
		},
	}
	for name, answer := range cases {
		t.Run(name, func(t *testing.T) {
			f := &retryFixture{answer: answer}
			_, err := f.retrier(time.Second).fetch(context.Background(), expiredImage(), retryInfo)
			if !errors.Is(err, ErrMediaGone) {
				t.Fatalf("erro %v", err)
			}
		})
	}
}

func TestDownloadTimesOutWhenThePhoneDoesNotAnswer(t *testing.T) {
	f := &retryFixture{}
	_, err := f.retrier(10*time.Millisecond).fetch(context.Background(), expiredImage(), retryInfo)
	if !errors.Is(err, ErrMediaRetryTimeout) {
		t.Fatalf("erro %v", err)
	}
}

func TestDownloadRejectsAnAnswerEncryptedWithAnotherKey(t *testing.T) {
	f := &retryFixture{}
	f.answer = func() *events.MediaRetry {
		evt := retryAnswer(t, "ABC", waMmsRetry.MediaRetryNotification_SUCCESS, "/novo")
		evt.Ciphertext[0] ^= 0xff
		return evt
	}
	_, err := f.retrier(time.Second).fetch(context.Background(), expiredImage(), retryInfo)
	if err == nil || errors.Is(err, ErrMediaGone) {
		t.Fatalf("erro %v", err)
	}
	if f.downloads.Load() != 1 {
		t.Fatal("resposta inválida não pode virar novo download")
	}
}

func TestSendFailureIsReported(t *testing.T) {
	f := &retryFixture{sendErr: errors.New("desconectado")}
	_, err := f.retrier(time.Second).fetch(context.Background(), expiredImage(), retryInfo)
	if !errors.Is(err, f.sendErr) {
		t.Fatalf("erro %v", err)
	}
}

func TestMediaMessageInfoValidation(t *testing.T) {
	bad := map[string]MediaMessageInfo{
		"sem id":           {Chat: "5511999999999@s.whatsapp.net"},
		"chat inválido":    {Id: "A", Chat: "@@"},
		"grupo sem autor":  {Id: "A", Chat: "123@g.us"},
		"grupo autor ruim": {Id: "A", Chat: "123@g.us", Sender: "@@"},
		"sem chat nenhum":  {Id: "A"},
	}
	for name, info := range bad {
		if _, err := info.toMessageInfo(); !errors.Is(err, ErrInvalidMediaInfo) {
			t.Errorf("%s: erro %v", name, err)
		}
	}

	good, err := (&MediaMessageInfo{Id: "A", Chat: "123@g.us", Sender: "5511999999999@s.whatsapp.net", FromMe: true}).toMessageInfo()
	if err != nil {
		t.Fatal(err)
	}
	if !good.IsGroup || !good.IsFromMe || good.Sender.IsEmpty() || good.ID != "A" {
		t.Fatalf("info %+v", good)
	}

	direct, err := (&MediaMessageInfo{Id: "B", Chat: "5511999999999@s.whatsapp.net"}).toMessageInfo()
	if err != nil {
		t.Fatal(err)
	}
	if direct.IsGroup || !direct.Sender.IsEmpty() {
		t.Fatalf("info %+v", direct)
	}
}

func TestSelectMediaCoversEveryKind(t *testing.T) {
	cases := map[string]*waE2E.Message{
		"image":    {ImageMessage: &waE2E.ImageMessage{Mimetype: proto.String("m")}},
		"audio":    {AudioMessage: &waE2E.AudioMessage{Mimetype: proto.String("m")}},
		"document": {DocumentMessage: &waE2E.DocumentMessage{Mimetype: proto.String("m")}},
		"video":    {VideoMessage: &waE2E.VideoMessage{Mimetype: proto.String("m")}},
		"sticker":  {StickerMessage: &waE2E.StickerMessage{Mimetype: proto.String("m")}},
	}
	for kind, msg := range cases {
		media, got, mimetype := selectMedia(msg)
		if media == nil || got != kind || mimetype != "m" {
			t.Errorf("%s: media=%v kind=%q mimetype=%q", kind, media, got, mimetype)
		}
		setDirectPath(media, "/caminho")
		if media.GetDirectPath() != "/caminho" {
			t.Errorf("%s: setDirectPath não gravou", kind)
		}
	}
	if media, _, _ := selectMedia(&waE2E.Message{Conversation: proto.String("oi")}); media != nil {
		t.Fatal("texto não tem mídia")
	}
}
