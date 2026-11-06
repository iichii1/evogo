package whatsmeow_service

import (
	"context"
	"errors"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/types/events"
)

// Quando o hash de uma coleção do app state deixa de bater com o do servidor, todo patch seguinte
// falha do mesmo jeito e o que o celular faz (favoritar, fixar conversa, ler) nunca chega. Foi o que
// o ensaio de 2026-11-06 mostrou: "failed to verify patch v28: mismatching LTHash". A saída é baixar a
// coleção inteira de novo, uma vez, e a partir daí os patches voltam a verificar.
const appStateResyncCooldown = 10 * time.Minute

type appStateResync struct {
	mu   sync.Mutex
	last map[appstate.WAPatchName]time.Time
}

// shouldResyncAppState diz se o erro pede uma nova sincronização inteira da coleção. Um erro da própria
// sincronização inteira não pede outra, para não entrar em laço.
func shouldResyncAppState(evt *events.AppStateSyncError) bool {
	return evt != nil && !evt.FullSync && errors.Is(evt.Error, appstate.ErrMismatchingLTHash)
}

// allow aceita no máximo uma nova sincronização por coleção em cada intervalo.
func (r *appStateResync) allow(name appstate.WAPatchName, now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if last, ok := r.last[name]; ok && now.Sub(last) < appStateResyncCooldown {
		return false
	}
	if r.last == nil {
		r.last = map[appstate.WAPatchName]time.Time{}
	}
	r.last[name] = now
	return true
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
		if err := mycli.WAClient.FetchAppState(ctx, evt.Name, true, false); err != nil {
			log.LogError("[%s] Não foi possível baixar o app state %s de novo: %s", mycli.userID, evt.Name, err)
			return
		}
		log.LogInfo("[%s] App state %s sincronizado de novo", mycli.userID, evt.Name)
	}()
}
