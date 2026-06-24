// Package estado gestiona el estado local de una expendedora:
// su inventario de productos y su lista de vetos activos.
// Todas las operaciones de escritura son protegidas por mutex
// para soportar acceso concurrente desde múltiples goroutines.
package estado

import (
	"fmt"
	"strings"
	"sync"
)

// Item representa un producto en el inventario de la expendedora.
type Item struct {
	Nombre   string `json:"nombre"`
	Cantidad int    `json:"cantidad"`
}

// Veto representa una persona vetada con su contador de instrucciones restantes.
// Cuando Counter llega a 0, la persona es perdonada automáticamente.
type Veto struct {
	Persona string
	Counter int
}

// Proceso encapsula el estado completo de una expendedora:
// su identidad, inventario replicado de todos los procesos,
// lista de vetos locales y bandera de infección.
type Proceso struct {
	mu sync.RWMutex

	Maquina int
	ID      int

	// inventario local de ESTE proceso (copia autoritativa)
	inventario []Item

	// replica de inventarios de todos los procesos del sistema
	// clave: "M<m>P<p>" -> lista de items
	replicaInventario map[string][]Item

	// vetos activos: persona en minúsculas -> counter
	vetos map[string]int

	// infectado: cuando true, este proceso enviará inventarios alterados
	infectado bool
}

// NuevoProceso crea un proceso con inventario inicial dado.
func NuevoProceso(maquina, id int, inventario []Item) *Proceso {
	p := &Proceso{
		Maquina:           maquina,
		ID:                id,
		inventario:        copiarItems(inventario),
		replicaInventario: make(map[string][]Item),
		vetos:             make(map[string]int),
	}
	clave := fmt.Sprintf("M%dP%d", maquina, id)
	p.replicaInventario[clave] = copiarItems(inventario)
	return p
}

// --- Inventario ---

// ObtenerInventario retorna una copia del inventario local.
func (p *Proceso) ObtenerInventario() []Item {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return copiarItems(p.inventario)
}

// ActualizarInventarioLocal reemplaza el inventario local del proceso.
// Se llama tras una compra exitosa o recuperación.
func (p *Proceso) ActualizarInventarioLocal(items []Item) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.inventario = copiarItems(items)
	clave := fmt.Sprintf("M%dP%d", p.Maquina, p.ID)
	p.replicaInventario[clave] = copiarItems(items)
}

// SetReplicaInventario guarda la réplica de inventario de otro proceso.
func (p *Proceso) SetReplicaInventario(maquina, proceso int, items []Item) {
	p.mu.Lock()
	defer p.mu.Unlock()
	clave := fmt.Sprintf("M%dP%d", maquina, proceso)
	p.replicaInventario[clave] = copiarItems(items)
}

// ObtenerReplicaInventario retorna la réplica del inventario de un proceso dado.
func (p *Proceso) ObtenerReplicaInventario(maquina, proceso int) ([]Item, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	clave := fmt.Sprintf("M%dP%d", maquina, proceso)
	items, ok := p.replicaInventario[clave]
	if !ok {
		return nil, false
	}
	return copiarItems(items), true
}

// InventarioParaEnvio retorna el inventario a enviar: si está infectado,
// devuelve una versión alterada (cantidades * 2 + 1) para simular corrupción.
func (p *Proceso) InventarioParaEnvio() []Item {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.infectado {
		return inventarioAlterado(p.inventario)
	}
	return copiarItems(p.inventario)
}

// --- Compra ---

// IntentarCompra verifica veto y stock y aplica la compra si es posible.
// Retorna "DENEGADO", "NO VALIDO" o "VALIDO".
// Si retorna "VALIDO", el inventario ya fue descontado.
func (p *Proceso) IntentarCompra(persona, producto string, cantidad int) string {
	p.mu.Lock()
	defer p.mu.Unlock()

	personaNorm := normalizarPersona(persona)
	if _, vetado := p.vetos[personaNorm]; vetado {
		return "DENEGADO"
	}

	// Buscar producto (case-insensitive)
	idx := p.buscarProducto(producto)
	if idx < 0 || p.inventario[idx].Cantidad < cantidad {
		return "NO VALIDO"
	}

	p.inventario[idx].Cantidad -= cantidad
	clave := fmt.Sprintf("M%dP%d", p.Maquina, p.ID)
	p.replicaInventario[clave] = copiarItems(p.inventario)
	return "VALIDO"
}

// buscarProducto retorna el índice del producto en el inventario o -1.
// DEBE llamarse con el mutex tomado.
func (p *Proceso) buscarProducto(nombre string) int {
	nombreNorm := strings.ToLower(strings.TrimSpace(nombre))
	for i, it := range p.inventario {
		if strings.ToLower(strings.TrimSpace(it.Nombre)) == nombreNorm {
			return i
		}
	}
	return -1
}

// --- Vetos ---

// VetarPersona añade o renueva el veto de una persona (counter = 5).
// Retorna true si era un veto nuevo, false si fue renovado.
func (p *Proceso) VetarPersona(persona string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	personaNorm := normalizarPersona(persona)
	_, existia := p.vetos[personaNorm]
	p.vetos[personaNorm] = 5
	return !existia
}

// PerdonarPersona elimina el veto de una persona.
// Retorna true si la persona estaba vetada.
func (p *Proceso) PerdonarPersona(persona string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	personaNorm := normalizarPersona(persona)
	_, existia := p.vetos[personaNorm]
	delete(p.vetos, personaNorm)
	return existia
}

// DecrementarCounterVetos reduce en 1 el counter de todos los vetos activos.
// Los vetos que llegan a 0 son perdonados automáticamente.
// Retorna lista de personas perdonadas automáticamente.
func (p *Proceso) DecrementarCounterVetos() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	var perdonados []string
	for persona, counter := range p.vetos {
		p.vetos[persona] = counter - 1
		if p.vetos[persona] <= 0 {
			delete(p.vetos, persona)
			perdonados = append(perdonados, persona)
		}
	}
	return perdonados
}

// SetVetosDesdeReplica reemplaza la lista de vetos completa (usado en sync y recuperación).
func (p *Proceso) SetVetosDesdeReplica(vetos map[string]int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.vetos = make(map[string]int, len(vetos))
	for k, v := range vetos {
		p.vetos[k] = v
	}
}

// ObtenerVetos retorna una copia del mapa de vetos actuales.
func (p *Proceso) ObtenerVetos() map[string]int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	copia := make(map[string]int, len(p.vetos))
	for k, v := range p.vetos {
		copia[k] = v
	}
	return copia
}

// CounterVeto retorna el counter actual de una persona vetada y si existe.
func (p *Proceso) CounterVeto(persona string) (int, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	personaNorm := normalizarPersona(persona)
	c, ok := p.vetos[personaNorm]
	return c, ok
}

// --- Infección ---

// ToggleInfeccion alterna el estado de infección del proceso.
// Cuando está infectado, los inventarios enviados a otros procesos serán alterados.
func (p *Proceso) ToggleInfeccion() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.infectado = !p.infectado
	return p.infectado
}

// EstaInfectado retorna si el proceso está en modo infectado.
func (p *Proceso) EstaInfectado() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.infectado
}

// --- Helpers ---

// normalizarPersona convierte el nombre de una persona a minúsculas para comparaciones.
func normalizarPersona(persona string) string {
	return strings.ToLower(strings.TrimSpace(persona))
}

// copiarItems hace una copia profunda de un slice de Items.
func copiarItems(src []Item) []Item {
	if src == nil {
		return []Item{}
	}
	dst := make([]Item, len(src))
	copy(dst, src)
	return dst
}

// inventarioAlterado retorna una versión del inventario con cantidades modificadas
// para simular la corrupción enviada por un proceso infectado.
// La alteración es: cantidad * 2 + 1 (visible pero no completamente diferente).
func inventarioAlterado(items []Item) []Item {
	alterado := make([]Item, len(items))
	for i, it := range items {
		alterado[i] = Item{
			Nombre:   it.Nombre,
			Cantidad: it.Cantidad*2 + 1,
		}
	}
	return alterado
}
