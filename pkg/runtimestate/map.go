// Package runtimestate guarda os registros em memória compartilhados entre os
// handlers HTTP e as goroutines de eventos do WhatsApp.
//
// Antes eles eram mapas Go comuns acessados de várias goroutines sem trava, o
// que causa "fatal error: concurrent map writes" e derruba o processo inteiro.
package runtimestate

import "sync"

// Map é um mapa seguro para uso concorrente.
//
// As travas cobrem só o acesso ao mapa, nunca operações de rede.
type Map[K comparable, V comparable] struct {
	mu     sync.RWMutex
	values map[K]V
	guards map[K]*keyGuard
}

// New cria um Map vazio.
func New[K comparable, V comparable]() *Map[K, V] {
	return &Map[K, V]{}
}

// Load devolve o valor e se ele existe.
func (m *Map[K, V]) Load(key K) (V, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.values[key]
	return value, ok
}

// Get devolve o valor ou o valor zero do tipo.
func (m *Map[K, V]) Get(key K) V {
	value, _ := m.Load(key)
	return value
}

// Set grava o valor.
func (m *Map[K, V]) Set(key K, value V) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.values == nil {
		m.values = make(map[K]V)
	}
	m.values[key] = value
}

// Delete remove a chave.
func (m *Map[K, V]) Delete(key K) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.values, key)
}

// CompareAndDelete remove a chave só se o valor atual for expected.
//
// Serve para que a limpeza de um cliente antigo não apague o cliente novo que
// já o substituiu.
func (m *Map[K, V]) CompareAndDelete(key K, expected V) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.values[key]
	if !ok || current != expected {
		return false
	}
	delete(m.values, key)
	return true
}

// Len devolve o número de chaves.
func (m *Map[K, V]) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.values)
}

// Values devolve uma cópia dos valores no momento da chamada. Como é uma cópia,
// quem a recebe pode percorrê-la, e chamar o próprio Map, sem segurar a trava.
func (m *Map[K, V]) Values() []V {
	m.mu.RLock()
	defer m.mu.RUnlock()
	values := make([]V, 0, len(m.values))
	for _, value := range m.values {
		values = append(values, value)
	}
	return values
}

type keyGuard struct {
	mu    sync.Mutex
	users int
}

// LockKey serializa operações sobre uma mesma chave (por exemplo, conectar e
// desconectar a mesma instância) sem travar as outras chaves. Devolve a função
// que libera a trava.
func (m *Map[K, V]) LockKey(key K) func() {
	m.mu.Lock()
	if m.guards == nil {
		m.guards = make(map[K]*keyGuard)
	}
	guard := m.guards[key]
	if guard == nil {
		guard = &keyGuard{}
		m.guards[key] = guard
	}
	guard.users++
	m.mu.Unlock()

	guard.mu.Lock()

	var once sync.Once
	return func() {
		once.Do(func() {
			guard.mu.Unlock()
			m.mu.Lock()
			defer m.mu.Unlock()
			guard.users--
			if guard.users == 0 {
				delete(m.guards, key)
			}
		})
	}
}
