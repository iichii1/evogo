package whatsmeow_service

import (
	"context"
	"errors"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/types/events"
)

// Quando o hash de uma coleção do app state deixa de bater com o do servidor, todo patch seguinte
// falha do mesmo jeito e o que o celular faz (favoritar, fixar conversa, ler) nunca chega. Foi o que
// o ensaio de 2026-11-06 mostrou: "failed to verify patch v28: mismatching LTHash". A saída é baixar a
// coleção inteira de novo, uma vez, e a partir daí os patches voltam a verificar.
//
// Nem sempre basta: no ensaio de 2026-11-06 a coleção baixada de novo falhou no mesmo patch v28, porque o
// problema não estava no estado guardado aqui. Aí o único caminho que sobra é pedir ao celular uma cópia
// da coleção (a recuperação que a whatsmeow já sabe receber). Pedir o reinício das coleções, que desconecta
// todos os aparelhos, não é feito aqui de propósito.
const (
	appStateResyncCooldown   = 10 * time.Minute
	appStateRecoveryCooldown = 30 * time.Minute
)

type appStateResync struct {
	mu       sync.Mutex
	last     map[appstate.WAPatchName]time.Time
	recovery map[appstate.WAPatchName]time.Time
}

// shouldResyncAppState diz se o erro pede uma nova sincronização inteira da coleção. Um erro da própria
// sincronização inteira não pede outra, para não entrar em laço.
func shouldResyncAppState(evt *events.AppStateSyncError) bool {
	return evt != nil && !evt.FullSync && errors.Is(evt.Error, appstate.ErrMismatchingLTHash)
}

// shouldRecoverAppState diz se a coleção baixada de novo falhou do mesmo jeito, e só então pede a cópia ao celular.
func shouldRecoverAppState(err error) bool {
	return errors.Is(err, appstate.ErrMismatchingLTHash)
}

// allow aceita no máximo uma nova sincronização por coleção em cada intervalo.
func (r *appStateResync) allow(name appstate.WAPatchName, now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return take(&r.last, name, now, appStateResyncCooldown)
}

// allowRecovery aceita no máximo um pedido de cópia ao celular por coleção em cada intervalo.
func (r *appStateResync) allowRecovery(name appstate.WAPatchName, now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return take(&r.recovery, name, now, appStateRecoveryCooldown)
}

func take(marks *map[appstate.WAPatchName]time.Time, name appstate.WAPatchName, now time.Time, interval time.Duration) bool {
	if last, ok := (*marks)[name]; ok && now.Sub(last) < interval {
		return false
	}
	if *marks == nil {
		*marks = map[appstate.WAPatchName]time.Time{}
	}
	(*marks)[name] = now
	return true
}

// resyncOrRecover baixa a coleção de novo e, se isso falhar com o mesmo hash que não bate, pede a cópia ao
// celular. Devolve o que fez, para o log e para o teste.
func (r *appStateResync) resyncOrRecover(
	ctx context.Context,
	name appstate.WAPatchName,
	now time.Time,
	fetch func(context.Context, appstate.WAPatchName) error,
	askPhone func(context.Context, appstate.WAPatchName) error,
) (string, error) {
	err := fetch(ctx, name)
	if err == nil {
		return "resynced", nil
	}
	if !shouldRecoverAppState(err) {
		return "failed", err
	}
	if !r.allowRecovery(name, now) {
		return "recovery-recently-asked", err
	}
	if askErr := askPhone(ctx, name); askErr != nil {
		return "recovery-not-sent", askErr
	}
	return "recovery-asked", err
}

// resyncAppState baixa de novo a coleção que falhou. Roda em goroutine porque o evento chega com o
// bloqueio da sincronização ainda preso; a busca espera ele soltar.
func (mycli *MyClient) resyncAppState(evt *events.AppStateSyncError) {
	if !shouldResyncAppState(evt) || mycli.WAClient == nil || !mycli.appStateResync.allow(evt.Name, time.Now()) {
		return
	}
	log := mycli.loggerWrapper.GetLogger(mycli.userID)
	log.LogWarn("[%s] App state %s fora de sincronia com o servidor; baixando a coleção de novo", mycli.userID, evt.Name)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		outcome, err := mycli.appStateResync.resyncOrRecover(ctx, evt.Name, time.Now(),
			func(ctx context.Context, name appstate.WAPatchName) error {
				return mycli.WAClient.FetchAppState(ctx, name, true, false)
			},
			func(ctx context.Context, name appstate.WAPatchName) error {
				_, err := mycli.WAClient.SendPeerMessage(ctx, whatsmeow.BuildAppStateRecoveryRequest(name))
				return err
			})
		switch outcome {
		case "resynced":
			log.LogInfo("[%s] App state %s sincronizado de novo", mycli.userID, evt.Name)
		case "recovery-asked":
			log.LogWarn("[%s] App state %s continua fora de sincronia depois de baixar de novo (%s); pedi uma cópia da coleção ao celular", mycli.userID, evt.Name, err)
		case "recovery-recently-asked":
			log.LogWarn("[%s] App state %s continua fora de sincronia; a cópia já foi pedida ao celular há pouco", mycli.userID, evt.Name)
		case "recovery-not-sent":
			log.LogError("[%s] Não foi possível pedir ao celular a cópia do app state %s: %s", mycli.userID, evt.Name, err)
		default:
			log.LogError("[%s] Não foi possível baixar o app state %s de novo: %s", mycli.userID, evt.Name, err)
		}
	}()
}
