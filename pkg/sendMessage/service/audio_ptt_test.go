package send_service

import (
	"bytes"
	"os/exec"
	"testing"
)

func TestMediaStructIsPTTDefaultsToVoiceNote(t *testing.T) {
	yes, no := true, false

	if !(&MediaStruct{}).IsPTT() {
		t.Fatal("audio without ptt should stay a voice note")
	}
	if !(&MediaStruct{Ptt: &yes}).IsPTT() {
		t.Fatal("ptt=true should be a voice note")
	}
	if (&MediaStruct{Ptt: &no}).IsPTT() {
		t.Fatal("ptt=false should be a regular audio")
	}
}

func TestOpusProfile(t *testing.T) {
	if ch, app := opusProfile(true); ch != "1" || app != "voip" {
		t.Fatalf("voice note profile = %s/%s", ch, app)
	}
	if ch, app := opusProfile(false); ch != "2" || app != "audio" {
		t.Fatalf("regular audio profile = %s/%s", ch, app)
	}
}

// TestConvertAudioRegularAudioIsStereo gera 1s de tom com ffmpeg e confere que
// a conversão com ptt=false produz Opus estéreo e com ptt=true, mono.
func TestConvertAudioRegularAudioIsStereo(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed")
	}

	var mp3 bytes.Buffer
	gen := exec.Command("ffmpeg", "-f", "lavfi", "-i", "sine=frequency=440:duration=1", "-ac", "2", "-f", "mp3", "pipe:1")
	gen.Stdout = &mp3
	if err := gen.Run(); err != nil {
		t.Skipf("could not generate sample mp3: %v", err)
	}

	channels := func(data []byte) string {
		probe := exec.Command("ffprobe", "-v", "error", "-select_streams", "a:0", "-show_entries", "stream=channels", "-of", "csv=p=0", "pipe:0")
		probe.Stdin = bytes.NewReader(data)
		out, err := probe.Output()
		if err != nil {
			t.Fatalf("ffprobe: %v", err)
		}
		return string(bytes.TrimSpace(out))
	}

	voice, duration, err := convertAudioToOpusWithDuration(mp3.Bytes(), true)
	if err != nil {
		t.Fatal(err)
	}
	if got := channels(voice); got != "1" {
		t.Fatalf("voice note channels = %s, want 1", got)
	}
	if duration != 1 {
		t.Fatalf("duration = %d, want 1", duration)
	}

	regular, _, err := convertAudioToOpusWithDuration(mp3.Bytes(), false)
	if err != nil {
		t.Fatal(err)
	}
	if got := channels(regular); got != "2" {
		t.Fatalf("regular audio channels = %s, want 2", got)
	}
}
