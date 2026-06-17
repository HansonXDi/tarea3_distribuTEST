package store

import (
	"sort"
	"sync"
	"tarea3/internal/protocol"
)

// vetoEvent representa un único evento de veto o perdón sobre una persona,
// con su reloj de Lamport asociado para poder ordenarlo causalmente respecto
// a otros eventos sobre la misma persona, sin importar en qué orden llegaron
// por la red.
type vetoEvent struct {
	clock   int64
	isVeto  bool // true = VETAR (fija counter=5), false = PERDONAR (elimina)
	counter int  // solo relevante si isVeto == true
}

// Store mantiene el estado local del proceso: inventario y lista de vetos.
// Es thread-safe mediante un RWMutex.
//
// La lista de vetos NO se aplica mensaje por mensaje al vuelo: cada evento
// (VETAR/PERDONAR, local o remoto) se guarda en una cola ordenada por reloj
// de Lamport (eventLog) y el estado visible (vetos) se recalcula re-jugando
// esa cola en orden. Así ningún evento se pierde aunque lleguen desordenados
// por la red, y el resultado final es siempre el mismo sin importar el orden
// de llegada.
type Store struct {
	mu        sync.RWMutex
	inventory []protocol.Item
	vetos     map[string]int          // persona -> counter vigente (derivado de eventLog)
	eventLog  map[string][]vetoEvent  // persona -> eventos ordenables por clock
	clock     int64                   // reloj de Lamport local de este proceso
}

// New crea un Store vacío.
// Entrada: ninguna. Salida: *Store inicializado.
func New() *Store {
	return &Store{
		vetos:    make(map[string]int),
		eventLog: make(map[string][]vetoEvent),
	}
}

// SetInventory reemplaza el inventario completo de forma atómica.
// Entrada: slice de Items. Salida: ninguna.
func (s *Store) SetInventory(items []protocol.Item) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]protocol.Item, len(items))
	copy(cp, items)
	s.inventory = cp
}

// GetInventory devuelve una copia del inventario actual.
// Entrada: ninguna. Salida: slice de Items.
func (s *Store) GetInventory() []protocol.Item {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp := make([]protocol.Item, len(s.inventory))
	copy(cp, s.inventory)
	return cp
}

// Buy descuenta cantidad de un producto si hay stock y la persona no está vetada.
// Entrada: persona, producto, cantidad. Salida: "VALIDO", "DENEGADO" o "NO VALIDO".
func (s *Store) Buy(persona, producto string, cantidad int) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, vetado := s.vetos[persona]; vetado {
		return "DENEGADO"
	}
	for i, item := range s.inventory {
		if item.Nombre == producto {
			if item.Cantidad < cantidad {
				return "NO VALIDO"
			}
			s.inventory[i].Cantidad -= cantidad
			return "VALIDO"
		}
	}
	return "NO VALIDO"
}

// Tick avanza el reloj de Lamport local en 1 y lo retorna. Se llama antes de
// cada evento local (VETAR/PERDONAR) para sellarlo con un clock único.
// Entrada: ninguna. Salida: el nuevo valor del reloj.
func (s *Store) Tick() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clock++
	return s.clock
}

// Observe actualiza el reloj de Lamport local al recibir un mensaje con un
// clock remoto: clock = max(local, remoto) + 1. Esto mantiene la propiedad
// de Lamport (todo evento causalmente posterior tiene un clock mayor).
// Entrada: clock recibido en el mensaje. Salida: ninguna.
func (s *Store) Observe(remoteClock int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if remoteClock > s.clock {
		s.clock = remoteClock
	}
	s.clock++
}

// applyVeto inserta un evento VETAR en la cola de esa persona y recalcula su
// estado vigente. No requiere lock externo: lo toma internamente.
// Entrada: persona, clock del evento, counter inicial (siempre 5 para VETAR
// nuevo, pero se deja explícito para reusar en sync). Salida: ninguna.
func (s *Store) applyVeto(persona string, clock int64, counter int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.insertEvent(persona, vetoEvent{clock: clock, isVeto: true, counter: counter})
}

// applyPardon inserta un evento PERDONAR en la cola de esa persona y
// recalcula su estado vigente.
// Entrada: persona, clock del evento. Salida: ninguna.
func (s *Store) applyPardon(persona string, clock int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.insertEvent(persona, vetoEvent{clock: clock, isVeto: false})
}

// insertEvent agrega un evento a la cola de una persona, la reordena por
// clock y recalcula el estado vigente (vetos[persona]) re-jugando la cola
// completa en orden. Requiere que el caller ya tenga el lock tomado.
// Entrada: persona, evento nuevo. Salida: ninguna.
func (s *Store) insertEvent(persona string, ev vetoEvent) {
	log := append(s.eventLog[persona], ev)
	sort.SliceStable(log, func(i, j int) bool {
		return log[i].clock < log[j].clock
	})
	s.eventLog[persona] = log

	// Re-jugar la cola completa en orden: el último evento determina el
	// estado vigente. Esto es intencionalmente simple (no hay "merge" de
	// counters, el último VETAR/PERDONAR por orden de clock gana), lo cual
	// es exactamente la semántica pedida por el enunciado: un VETAR siempre
	// reinicia el counter a 5 y un PERDONAR siempre elimina el veto,
	// cualquiera sea el evento anterior.
	last := log[len(log)-1]
	if last.isVeto {
		s.vetos[persona] = last.counter
	} else {
		delete(s.vetos, persona)
	}
}

// Veto añade o reinicia el counter de veto de una persona como evento LOCAL.
// Entrada: persona. Salida: counter resultante (siempre 5) y el clock usado.
func (s *Store) Veto(persona string) (int, int64) {
	clock := s.Tick()
	s.applyVeto(persona, clock, 5)
	return 5, clock
}

// Pardon elimina el veto de una persona como evento LOCAL.
// Entrada: persona. Salida: clock usado para este evento.
func (s *Store) Pardon(persona string) int64 {
	clock := s.Tick()
	s.applyPardon(persona, clock)
	return clock
}

// DecrementVetos reduce en 1 todos los counters activos y elimina (perdona)
// los que llegan a 0. Cada perdón automático se registra como un evento más
// en la cola de esa persona, con su propio clock local.
// Entrada: ninguna. Salida: slice de (persona, clock) perdonados automáticamente.
func (s *Store) DecrementVetos() []PardonedEvent {
	s.mu.Lock()
	current := make(map[string]int, len(s.vetos))
	for p, c := range s.vetos {
		current[p] = c
	}
	s.mu.Unlock()

	var pardoned []PardonedEvent
	for p, c := range current {
		c--
		if c <= 0 {
			clock := s.Pardon(p)
			pardoned = append(pardoned, PardonedEvent{Persona: p, Clock: clock})
		} else {
			clock := s.Tick()
			s.applyVeto(p, clock, c)
		}
	}
	return pardoned
}

// PardonedEvent identifica a una persona perdonada automáticamente junto al
// reloj de Lamport con el que se selló ese perdón, para poder propagarlo.
type PardonedEvent struct {
	Persona string
	Clock   int64
}

// GetVetos devuelve la lista de vetos activos como slice.
// Entrada: ninguna. Salida: slice de VetoEntry.
func (s *Store) GetVetos() []protocol.VetoEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]protocol.VetoEntry, 0, len(s.vetos))
	for p, c := range s.vetos {
		out = append(out, protocol.VetoEntry{Persona: p, Counter: c})
	}
	return out
}

// SetVetos reemplaza la lista de vetos completa (usado en sincronización
// periódica). Se trata como un evento único de alto clock para que domine
// sobre el historial previo de cada persona involucrada.
// Entrada: slice de VetoEntry, clock del mensaje de sincronización. Salida: ninguna.
func (s *Store) SetVetos(entries []protocol.VetoEntry, clock int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range entries {
		s.insertEvent(e.Persona, vetoEvent{clock: clock, isVeto: true, counter: e.Counter})
	}
}

// IsVetoed informa si una persona está vetada actualmente.
// Entrada: nombre de persona. Salida: bool.
func (s *Store) IsVetoed(persona string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.vetos[persona]
	return ok
}

// UpdateVeto aplica un veto recibido de otro proceso (evento REMOTO). Nunca
// se descarta: se inserta en la cola de esa persona en su posición correcta
// según el clock, y el estado vigente se recalcula re-jugando toda la cola.
// Entrada: persona, counter recibido, clock del mensaje. Salida: ninguna.
func (s *Store) UpdateVeto(persona string, counter int, clock int64) {
	s.applyVeto(persona, clock, counter)
}

// PardonRemote aplica un perdón recibido de otro proceso (evento REMOTO), con
// la misma garantía de no pérdida que UpdateVeto.
// Entrada: persona, clock del mensaje. Salida: ninguna.
func (s *Store) PardonRemote(persona string, clock int64) {
	s.applyPardon(persona, clock)
}
