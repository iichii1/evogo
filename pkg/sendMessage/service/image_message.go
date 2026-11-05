package send_service

import (
	"bytes"
	"image"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

// imageDimensions devolve a largura e a altura da imagem sem decodificar os
// pixels. Devolve 0, 0 se o formato não é reconhecido.
func imageDimensions(fileData []byte) (width, height uint32) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(fileData))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 {
		return 0, 0
	}
	return uint32(cfg.Width), uint32(cfg.Height)
}

func optionalUint32(v uint32) *uint32 {
	if v == 0 {
		return nil
	}
	return proto.Uint32(v)
}

// newImageMessage monta a ImageMessage de um envio de imagem já enviada à
// mídia do WhatsApp.
//
// Largura e altura são o que o WhatsApp usa para desenhar o balão antes de
// baixar a imagem inteira; sem elas o destinatário vê um quadrado cortado a
// partir da miniatura até a mídia carregar (#104). A miniatura JPEG vem do
// mesmo arquivo; se a imagem não puder ser decodificada, a mensagem sai sem
// ela em vez de falhar. Canais (newsletter) não são criptografados, então não
// levam MediaKey nem FileEncSHA256.
func newImageMessage(fileData []byte, uploaded whatsmeow.UploadResponse, mimeType, caption string, newsletter bool) *waE2E.ImageMessage {
	width, height := imageDimensions(fileData)
	msg := &waE2E.ImageMessage{
		Caption:       proto.String(caption),
		URL:           proto.String(uploaded.URL),
		DirectPath:    proto.String(uploaded.DirectPath),
		Mimetype:      proto.String(mimeType),
		FileSHA256:    uploaded.FileSHA256,
		JPEGThumbnail: makeJPEGThumbnail(fileData, 72),
		Width:         optionalUint32(width),
		Height:        optionalUint32(height),
	}
	if newsletter {
		msg.FileLength = proto.Uint64(uploaded.FileLength)
		return msg
	}
	msg.MediaKey = uploaded.MediaKey
	msg.FileEncSHA256 = uploaded.FileEncSHA256
	msg.FileLength = proto.Uint64(uint64(len(fileData)))
	return msg
}
