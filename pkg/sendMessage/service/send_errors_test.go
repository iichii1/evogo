package send_service

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"go.mau.fi/whatsmeow"
)

func TestExplainSendError(t *testing.T) {
	if explainSendError(nil) != nil {
		t.Fatal("nil should stay nil")
	}

	plain := errors.New("message 3EB0463 failed: context deadline exceeded")
	if explainSendError(plain) != plain {
		t.Fatal("unrelated errors should be returned unchanged")
	}

	other := fmt.Errorf("%w %d", whatsmeow.ErrServerReturnedError, 479)
	if explainSendError(other) != other {
		t.Fatal("other server error codes should be returned unchanged")
	}

	base := fmt.Errorf("%w %d", whatsmeow.ErrServerReturnedError, 463)
	got := explainSendError(base)
	if !errors.Is(got, whatsmeow.ErrServerReturnedError) {
		t.Fatal("explained error must wrap the original")
	}
	if !strings.Contains(got.Error(), "has not messaged this number first") {
		t.Fatalf("missing explanation: %v", got)
	}
}
