package logger

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Este teste varre o código-fonte atrás de linhas de log que gravam um segredo
// (token de instância, chave global, senha). Foi assim que o token de cada
// instância foi parar no log em ClearInstanceCache e a chave global quase inteira
// no log da conexão WebSocket.
//
// É uma varredura por linha: pega a forma comum, uma chamada de log numa só
// linha. Não substitui revisão, mas impede que o mesmo erro volte sem ninguém
// perceber.

var (
	logCall = regexp.MustCompile(`\b(Log(Info|Warn|Error|Debug)|Printf|Println|Fatalf|Errorf)\(`)

	// "Token: %s", "Token inválido: %s", "apikey=%v", "password %s": uma palavra de
	// segredo e, antes de fechar o texto, um verbo de formato.
	secretInFormat = regexp.MustCompile(`(?i)(token|api[ _-]?key|password|secret)[^"%]*%[-+#0-9.]*[svqxX]`)

	// Argumentos que são, por definição, segredos.
	secretArgument = regexp.MustCompile(`,\s*[\w.\[\]()]*(\.Token|\.Password|\.ProxyPassword|GlobalApiKey|\.AmqpUrl|\.NatsUrl|\.PostgresAuthDB)\b`)
)

// logsASecret informa se a linha chama uma função de log e grava um segredo.
func logsASecret(line string) bool {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "//") || !logCall.MatchString(line) {
		return false
	}
	return secretInFormat.MatchString(line) || secretArgument.MatchString(line)
}

func TestLogsASecretRecognizesTheKnownLeaks(t *testing.T) {
	leaks := []string{
		`w.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Clearing instance cache - Token: %s", instanceId, token)`,
		`mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] UserInfo cache cleared for token: %s", mycli.userID, mycli.Instance.Token)`,
		`logger.LogError("Token inválido: %s", token)`,
		`log.Printf("conectando com %s", cfg.GlobalApiKey)`,
	}
	for _, line := range leaks {
		if !logsASecret(line) {
			t.Errorf("deveria ser reconhecida como vazamento: %s", line)
		}
	}

	safe := []string{
		`w.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Clearing instance cache", instanceId)`,
		`logger.LogError("Conexão WebSocket recusada: token inválido")`,
		`mycli.loggerWrapper.GetLogger(id).LogWarn("[%s] Message type discarded: %s", id, arg)`,
		`// logger.LogError("Token inválido: %s", token)`,
		`return fmt.Errorf("proxy host is required")`,
	}
	for _, line := range safe {
		if logsASecret(line) {
			t.Errorf("não deveria ser reconhecida como vazamento: %s", line)
		}
	}
}

func TestNoSecretIsLoggedInTheSource(t *testing.T) {
	var offenders []string

	for _, dir := range []string{"../../pkg", "../../cmd"} {
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			// pkg/core é o gate de licença: não é código nosso para editar.
			if d.IsDir() && filepath.ToSlash(path) == "../../pkg/core" {
				return filepath.SkipDir
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for i, line := range strings.Split(string(src), "\n") {
				if logsASecret(line) {
					offenders = append(offenders, filepath.ToSlash(path)+":"+strconv.Itoa(i+1)+": "+strings.TrimSpace(line))
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	if len(offenders) > 0 {
		t.Fatalf("linhas de log que gravam um segredo (tire o valor do log):\n  %s", strings.Join(offenders, "\n  "))
	}
}
