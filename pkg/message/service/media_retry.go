package message_service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waMmsRetry"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// mediaRetryTimeout é quanto a requisição espera o celular responder. O Whatsflow
// espera 60 s por um download, então a resposta de erro chega antes do limite dele.
const mediaRetryTimeout = 30 * time.Second

var (
	// ErrMediaGone diz que o aparelho não tem mais a mídia: não adianta repetir.
	ErrMediaGone = errors.New("the phone no longer has this media")
	// ErrMediaRetryTimeout diz que o celular não respondeu a tempo; costuma ser um
	// aparelho desligado ou sem rede, então vale tentar de novo mais tarde.
	ErrMediaRetryTimeout = errors.New("the phone did not answer the media request in time")
	// ErrInvalidMediaInfo diz que os dados da mensagem vieram incompletos.
	ErrInvalidMediaInfo = errors.New("invalid media message info")
)

// MediaMessageInfo identifica a mensagem cuja mídia será pedida de novo ao aparelho.
type MediaMessageInfo struct {
	Id     string `json:"id"`
	Chat   string `json:"chat"`
	Sender string `json:"sender,omitempty"` // obrigatório em grupos
	FromMe bool   `json:"fromMe"`
}

func (i *MediaMessageInfo) toMessageInfo() (*types.MessageInfo, error) {
	if i.Id == "" {
		return nil, fmt.Errorf("%w: id is required", ErrInvalidMediaInfo)
	}
	chat, err := types.ParseJID(i.Chat)
	if err != nil || chat.IsEmpty() {
		return nil, fmt.Errorf("%w: invalid chat", ErrInvalidMediaInfo)
	}
	info := &types.MessageInfo{ID: i.Id}
	info.Chat = chat
	info.IsFromMe = i.FromMe
	info.IsGroup = chat.Server == types.GroupServer
	if info.IsGroup {
		sender, err := types.ParseJID(i.Sender)
		if err != nil || sender.IsEmpty() {
			return nil, fmt.Errorf("%w: sender is required in groups", ErrInvalidMediaInfo)
		}
		info.Sender = sender
	}
	return info, nil
}

// selectMedia devolve a mídia da mensagem, o nome do tipo para as mensagens de erro
// e o mimetype. A mídia é nil se a mensagem não tem nada para baixar.
func selectMedia(msg *waE2E.Message) (whatsmeow.DownloadableMessage, string, string) {
	switch {
	case msg.GetImageMessage() != nil:
		return msg.GetImageMessage(), "image", msg.GetImageMessage().GetMimetype()
	case msg.GetAudioMessage() != nil:
		return msg.GetAudioMessage(), "audio", msg.GetAudioMessage().GetMimetype()
	case msg.GetDocumentMessage() != nil:
		return msg.GetDocumentMessage(), "document", msg.GetDocumentMessage().GetMimetype()
	case msg.GetVideoMessage() != nil:
		return msg.GetVideoMessage(), "video", msg.GetVideoMessage().GetMimetype()
	case msg.GetStickerMessage() != nil:
		return msg.GetStickerMessage(), "sticker", msg.GetStickerMessage().GetMimetype()
	}
	return nil, "", ""
}

func setDirectPath(media whatsmeow.DownloadableMessage, path string) {
	switch m := media.(type) {
	case *waE2E.ImageMessage:
		m.DirectPath = &path
	case *waE2E.AudioMessage:
		m.DirectPath = &path
	case *waE2E.DocumentMessage:
		m.DirectPath = &path
	case *waE2E.VideoMessage:
		m.DirectPath = &path
	case *waE2E.StickerMessage:
		m.DirectPath = &path
	}
}

// mediaRetrier baixa uma mídia e, se o WhatsApp já a apagou, pede ao aparelho que
// a envie de novo. As dependências são funções para o fluxo ser testado sem um
// cliente conectado.
type mediaRetrier struct {
	send     func(ctx context.Context, info *types.MessageInfo, mediaKey []byte) error
	wait     func(messageID string) (<-chan *events.MediaRetry, func())
	download func(ctx context.Context, media whatsmeow.DownloadableMessage) ([]byte, error)
	timeout  time.Duration
}

func (r mediaRetrier) fetch(ctx context.Context, media whatsmeow.DownloadableMessage, info *types.MessageInfo) ([]byte, error) {
	data, err := r.download(ctx, media)
	if err == nil || info == nil || !isMediaExpired(err) {
		return data, err
	}

	path, err := r.requestDirectPath(ctx, info, media.GetMediaKey())
	if err != nil {
		return nil, err
	}
	setDirectPath(media, path)
	return r.download(ctx, media)
}

func isMediaExpired(err error) bool {
	return errors.Is(err, whatsmeow.ErrMediaDownloadFailedWith404) || errors.Is(err, whatsmeow.ErrMediaDownloadFailedWith410)
}

// requestDirectPath pede o reenvio e espera o novo caminho da mídia. O registro
// vem antes do pedido para a resposta não chegar sem ninguém esperando por ela.
func (r mediaRetrier) requestDirectPath(ctx context.Context, info *types.MessageInfo, mediaKey []byte) (string, error) {
	response, cancel := r.wait(info.ID)
	defer cancel()

	if err := r.send(ctx, info, mediaKey); err != nil {
		return "", fmt.Errorf("failed to request the media from the phone: %w", err)
	}

	timer := time.NewTimer(r.timeout)
	defer timer.Stop()
	select {
	case evt := <-response:
		return directPathFromRetry(evt, mediaKey)
	case <-timer.C:
		return "", ErrMediaRetryTimeout
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func directPathFromRetry(evt *events.MediaRetry, mediaKey []byte) (string, error) {
	notification, err := whatsmeow.DecryptMediaRetryNotification(evt, mediaKey)
	if errors.Is(err, whatsmeow.ErrMediaNotAvailableOnPhone) {
		return "", ErrMediaGone
	}
	if err != nil {
		return "", err
	}
	switch notification.GetResult() {
	case waMmsRetry.MediaRetryNotification_SUCCESS:
		if notification.GetDirectPath() == "" {
			return "", errors.New("the phone answered the media request without a new path")
		}
		return notification.GetDirectPath(), nil
	case waMmsRetry.MediaRetryNotification_NOT_FOUND:
		return "", ErrMediaGone
	default:
		return "", fmt.Errorf("the phone refused the media request: %s", notification.GetResult())
	}
}
