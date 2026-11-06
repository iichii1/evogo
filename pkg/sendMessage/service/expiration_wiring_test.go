package send_service

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// SendMessage exige um cliente conectado, então a ligação é conferida no código:
// toda chamada de um envio que aceita `expiration` precisa repassar o campo. Uma
// chamada esquecida faria a mensagem sair sem prazo, sem erro nenhum.
func TestEverySendPassesExpirationAlong(t *testing.T) {
	src, err := os.ReadFile("send_service.go")
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ReplaceAll(string(src), "\r\n", "\n")

	expected := map[string]bool{
		"sendTextWithRetry": true, "sendLinkWithRetry": true, "sendMediaFileWithRetry": true,
		"sendMediaUrlWithRetry": true, "sendPollWithRetry": true, "SendSticker": true,
		"SendLocation": true, "SendContact": true, "SendContacts": true,
		"SendGroupInvite": true, "SendRequestPhone": true,
	}
	funcs := regexp.MustCompile(`(?m)^func \(s \*sendService\) (\w+)\(`)
	matches := funcs.FindAllStringSubmatchIndex(text, -1)
	found := 0
	for i, m := range matches {
		name := text[m[2]:m[3]]
		if !expected[name] {
			continue
		}
		found++
		end := len(text)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		if !regexp.MustCompile(`Expiration:\s+data\.Expiration`).MatchString(text[m[0]:end]) {
			t.Errorf("%s não repassa data.Expiration para SendDataStruct", name)
		}
	}
	if found != len(expected) {
		t.Fatalf("achei %d de %d funções de envio: alguma mudou de nome", found, len(expected))
	}
}

// Um tipo novo fora dos dois switch de SendMessage (com e sem citação) responde
// "invalid messageType" só na hora de enviar.
func TestSendMessageKnowsTheNewTypes(t *testing.T) {
	src, err := os.ReadFile("send_service.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"ContactsArrayMessage", "GroupInviteMessage", "RequestPhoneNumberMessage"} {
		if got := strings.Count(string(src), `case "`+kind+`":`); got != 2 {
			t.Errorf("%s aparece em %d switch, esperava 2", kind, got)
		}
	}
}
