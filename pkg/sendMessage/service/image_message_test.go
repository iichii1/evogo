package send_service

import (
	"bytes"
	"image"
	"testing"

	"go.mau.fi/whatsmeow"
)

func fakeUpload() whatsmeow.UploadResponse {
	return whatsmeow.UploadResponse{
		URL:           "https://mmg.whatsapp.net/img",
		DirectPath:    "/v/img",
		MediaKey:      []byte("key"),
		FileEncSHA256: []byte("enc"),
		FileSHA256:    []byte("sha"),
		FileLength:    1234,
	}
}

// #104: sem largura e altura o WhatsApp mostra um quadrado até baixar a mídia.
func TestNewImageMessageSetsWidthHeightAndThumbnail(t *testing.T) {
	src := encodePNG(t, 1600, 720)

	msg := newImageMessage(src, fakeUpload(), "image/png", "legenda", false)

	if msg.GetWidth() != 1600 || msg.GetHeight() != 720 {
		t.Fatalf("dimensões = %dx%d, esperado 1600x720", msg.GetWidth(), msg.GetHeight())
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(msg.GetJPEGThumbnail()))
	if err != nil {
		t.Fatalf("sem miniatura JPEG válida: %v", err)
	}
	// A miniatura tem a mesma proporção da imagem (1600x720 -> 72x32).
	if cfg.Width != 72 || cfg.Height != 32 {
		t.Errorf("miniatura = %dx%d, esperado 72x32", cfg.Width, cfg.Height)
	}
	if msg.GetCaption() != "legenda" || msg.GetMimetype() != "image/png" {
		t.Errorf("legenda/mimetype perdidos: %q %q", msg.GetCaption(), msg.GetMimetype())
	}
	if string(msg.GetMediaKey()) != "key" || string(msg.GetFileEncSHA256()) != "enc" || string(msg.GetFileSHA256()) != "sha" {
		t.Error("a mídia criptografada precisa levar MediaKey, FileEncSHA256 e FileSHA256")
	}
	if msg.GetFileLength() != uint64(len(src)) {
		t.Errorf("FileLength = %d, esperado %d", msg.GetFileLength(), len(src))
	}
}

func TestNewImageMessageForNewsletterIsNotEncrypted(t *testing.T) {
	src := encodePNG(t, 100, 50)

	msg := newImageMessage(src, fakeUpload(), "image/png", "", true)

	if msg.MediaKey != nil || msg.FileEncSHA256 != nil {
		t.Error("canal (newsletter) não leva MediaKey nem FileEncSHA256")
	}
	if msg.GetFileLength() != 1234 {
		t.Errorf("FileLength = %d, esperado o do upload (1234)", msg.GetFileLength())
	}
	if msg.GetWidth() != 100 || msg.GetHeight() != 50 {
		t.Errorf("dimensões = %dx%d, esperado 100x50", msg.GetWidth(), msg.GetHeight())
	}
}

// Imagem que o Go não decodifica: a mensagem sai sem dimensões nem miniatura,
// em vez de falhar o envio.
func TestNewImageMessageWithUndecodableImageStillBuilds(t *testing.T) {
	msg := newImageMessage([]byte("isto não é uma imagem"), fakeUpload(), "image/jpeg", "x", false)

	if msg.Width != nil || msg.Height != nil {
		t.Errorf("dimensões deveriam ficar vazias, vieram %v x %v", msg.Width, msg.Height)
	}
	if msg.JPEGThumbnail != nil {
		t.Error("miniatura deveria ficar vazia")
	}
	if msg.GetURL() == "" || msg.GetDirectPath() == "" {
		t.Error("a mensagem perdeu a URL da mídia")
	}
}

func TestImageDimensions(t *testing.T) {
	if w, h := imageDimensions(encodePNG(t, 640, 480)); w != 640 || h != 480 {
		t.Errorf("imageDimensions = %dx%d, esperado 640x480", w, h)
	}
	if w, h := imageDimensions(nil); w != 0 || h != 0 {
		t.Errorf("imageDimensions(nil) = %dx%d, esperado 0x0", w, h)
	}
}
