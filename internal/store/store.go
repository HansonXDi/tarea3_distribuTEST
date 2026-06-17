package store

import (
	"sync"
	"tarea3/internal/protocol"
)

// Store mantiene el estado local del proceso: inventario y lista de vetos.
// Es thread-safe mediante un RWMutex.
type Store struct {
	mu        sync.RWMutex
	inventory []protocol.Item
	vetos     map[string]int // persona -> counter
}

// New crea un Store vacío.
// Entrada: ninguna. Salida: *Store inicializado.
func New() *Store {
	return &Store{vetos: make(map[string]int)}
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

// Veto añade o reinicia el counter de veto de una persona.
// Entrada: nombre de persona. Salida: counter resultante.
func (s *Store) Veto(persona string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vetos[persona] = 5
	return 5
}

// Pardon elimina el veto de una persona.
// Entrada: nombre de persona. Salida: ninguna.
func (s *Store) Pardon(persona string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.vetos, persona)
}

// DecrementVetos reduce en 1 todos los counters activos y elimina los que llegan a 0.
// Entrada: ninguna. Salida: slice de personas perdonadas automáticamente.
func (s *Store) DecrementVetos() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var pardoned []string
	for p, c := range s.vetos {
		c--
		if c <= 0 {
			delete(s.vetos, p)
			pardoned = append(pardoned, p)
		} else {
			s.vetos[p] = c
		}
	}
	return pardoned
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

// SetVetos reemplaza la lista de vetos completa (usado en sincronización).
// Entrada: slice de VetoEntry. Salida: ninguna.
func (s *Store) SetVetos(entries []protocol.VetoEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vetos = make(map[string]int, len(entries))
	for _, e := range entries {
		s.vetos[e.Persona] = e.Counter
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

// UpdateVeto aplica un veto remoto, respetando el counter recibido.
// Entrada: persona, counter. Salida: ninguna.
func (s *Store) UpdateVeto(persona string, counter int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vetos[persona] = counter
}
