package process

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"tarea3/internal/protocol"
	"tarea3/internal/store"
	netsync "tarea3/internal/sync"
	"time"
)

// Process representa una expendedora: tiene estado local y se comunica con sus pares.
type Process struct {
	MachineID   int
	ProcessID   int
	instructFile string
	st           *store.Store
	net          *netsync.Network
	malicious    bool
	mu           sync.Mutex
	logPath      string
	vetoLogPath  string
	instrCounter int // instrucciones ejecutadas (para decrement de vetos)
}

// New crea un Process y carga el inventario inicial aleatorio.
// Entrada: machineID, processID, archivo de instrucciones, peers, puerto de escucha.
// Salida: *Process listo para iniciar.
func New(machineID, processID int, instructFile string, peers []netsync.PeerConfig, listenPort int) *Process {
	p := &Process{
		MachineID:    machineID,
		ProcessID:    processID,
		instructFile: instructFile,
		st:           store.New(),
		logPath:      fmt.Sprintf("logs/inventario_M%dP%d.log", machineID, processID),
		vetoLogPath:  fmt.Sprintf("logs/vetos_M%dP%d.log", machineID, processID),
	}
	p.net = netsync.NewNetwork(machineID, processID, listenPort, peers, &p.malicious)

	// Carga un inventario aleatorio de la carpeta inventario/
	if err := p.loadRandomInventory(); err != nil {
		log.Fatalf("[P%d] No se pudo cargar inventario: %v", processID, err)
	}
	return p
}

// loadRandomInventory elige aleatoriamente un JSON de /inventario y lo carga en el store.
// Entrada: ninguna. Salida: error si no hay archivos o no se puede parsear.
func (p *Process) loadRandomInventory() error {
	matches, err := filepath.Glob("inventario/*.json")
	if err != nil || len(matches) == 0 {
		return fmt.Errorf("no hay archivos de inventario")
	}
	chosen := matches[rand.Intn(len(matches))]
	data, err := os.ReadFile(chosen)
	if err != nil {
		return err
	}
	var items []protocol.Item
	if err := json.Unmarshal(data, &items); err != nil {
		return err
	}
	p.st.SetInventory(items)
	log.Printf("[P%d] Inventario cargado desde %s", p.ProcessID, chosen)
	return nil
}

// Start inicia la escucha de red, conecta peers, espera que estén listos y ejecuta instrucciones.
// Entrada: ninguna. Salida: ninguna (bloquea hasta terminar instrucciones).
func (p *Process) Start() {
	if err := p.net.Listen(); err != nil {
		log.Fatalf("[P%d] Error escuchando: %v", p.ProcessID, err)
	}
	p.net.ConnectPeers()

	// Esperar hasta 2 segundos a que los peers conecten
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if p.net.ConnectedCount() >= len(p.getPeerAddrs()) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	log.Printf("[P%d] Conectado a %d peers. Iniciando instrucciones.", p.ProcessID, p.net.ConnectedCount())

	// Goroutine que procesa mensajes entrantes
	go p.handleMessages()

	// Ejecutar instrucciones del archivo asignado
	p.runInstructions()
}

// getPeerAddrs retorna las direcciones de todos los peers configurados.
// Entrada: ninguna. Salida: slice de strings con las IPs:puerto.
func (p *Process) getPeerAddrs() []string {
	// Esta función es un helper; la lógica real está en Network.
	return nil
}

// handleMessages procesa de forma continua los mensajes del canal de red.
// Entrada: ninguna. Salida: ninguna (loop infinito en goroutine).
func (p *Process) handleMessages() {
	for msg := range p.net.RecvCh {
		switch msg.Type {
		case protocol.MsgInventory:
			p.st.SetInventory(msg.Inventory)

		case protocol.MsgMalicious:
			log.Printf("[P%d] Inventario corrupto recibido de M%dP%d, ignorado.", p.ProcessID, msg.MachineID, msg.ProcessID)

		case protocol.MsgVeto:
			p.st.UpdateVeto(msg.VetoName, msg.Counter)
			p.writeVetoLog()

		case protocol.MsgPardon:
			p.st.Pardon(msg.VetoName)
			p.writeVetoLog()

		case protocol.MsgSyncVetos:
			p.st.SetVetos(msg.Vetos)
			p.writeVetoLog()

		case protocol.MsgRecoverReply:
			// Los replies de recovery se manejan en Recover()
			p.net.RecvCh <- msg

		case protocol.MsgHello:
			log.Printf("[P%d] HELLO de M%dP%d", p.ProcessID, msg.MachineID, msg.ProcessID)
		}
	}
}

// runInstructions lee y ejecuta cada línea del archivo de instrucciones asignado.
// Entrada: ninguna. Salida: ninguna.
func (p *Process) runInstructions() {
	f, err := os.Open(p.instructFile)
	if err != nil {
		log.Printf("[P%d] No se pudo abrir %s: %v", p.ProcessID, p.instructFile, err)
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		result := p.execInstruction(line)
		p.appendLog(line, result)
		p.instrCounter++
	}
}

// execInstruction parsea y ejecuta una instrucción de texto.
// Entrada: línea de instrucción. Salida: resultado ("VALIDO", "DENEGADO", "NO VALIDO", o "").
func (p *Process) execInstruction(line string) string {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return ""
	}
	cmd := strings.ToUpper(parts[0])

	switch cmd {
	case "VETAR":
		if len(parts) < 2 {
			return ""
		}
		persona := strings.Join(parts[1:], " ")
		p.mu.Lock()
		counter := p.st.Veto(persona)
		p.mu.Unlock()
		p.net.Broadcast(protocol.Message{
			Type:      protocol.MsgVeto,
			MachineID: p.MachineID,
			ProcessID: p.ProcessID,
			VetoName:  persona,
			Counter:   counter,
		})
		p.writeVetoLog()
		return ""

	case "COMPRAR":
		// Formato: COMPRAR <persona(s)> <producto> <cantidad>
		if len(parts) < 4 {
			return "NO VALIDO"
		}
		cantidad := 0
		fmt.Sscanf(parts[len(parts)-1], "%d", &cantidad)
		producto := parts[len(parts)-2]
		persona := strings.Join(parts[1:len(parts)-2], " ")

		p.mu.Lock()
		result := p.st.Buy(persona, producto, cantidad)
		p.mu.Unlock()

		if result == "VALIDO" {
			p.net.BroadcastInventory(p.st.GetInventory())
		}
		// Decrementar vetos cada instrucción
		pardoned := p.st.DecrementVetos()
		for _, pr := range pardoned {
			p.net.Broadcast(protocol.Message{
				Type:      protocol.MsgPardon,
				MachineID: p.MachineID,
				ProcessID: p.ProcessID,
				VetoName:  pr,
			})
		}
		p.writeVetoLog()
		return result

	case "PERDONAR":
		if len(parts) < 2 {
			return ""
		}
		persona := strings.Join(parts[1:], " ")
		p.mu.Lock()
		p.st.Pardon(persona)
		p.mu.Unlock()
		p.net.Broadcast(protocol.Message{
			Type:      protocol.MsgPardon,
			MachineID: p.MachineID,
			ProcessID: p.ProcessID,
			VetoName:  persona,
		})
		p.writeVetoLog()
		return ""
	}
	return ""
}

// Recover ejecuta el protocolo de recuperación: solicita inventarios a peers y elige por mayoría.
// Entrada: ninguna. Salida: error si no se alcanza quórum de 2/3.
func (p *Process) Recover() error {
	log.Printf("[P%d] Iniciando recuperación...", p.ProcessID)

	// Canal temporal para recolectar inventarios durante 3 segundos
	invCh := make(chan []protocol.Item, 64)

	// Interceptar mensajes de inventario por 3 segundos
	go func() {
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			select {
			case msg := <-p.net.RecvCh:
				if msg.Type == protocol.MsgInventory {
					invCh <- msg.Inventory
				}
			case <-time.After(100 * time.Millisecond):
			}
		}
		close(invCh)
	}()

	// Pedir a todos los peers que envíen su inventario
	p.net.Broadcast(protocol.Message{
		Type:      protocol.MsgRecover,
		MachineID: p.MachineID,
		ProcessID: p.ProcessID,
	})

	// Recolectar respuestas
	var received [][]protocol.Item
	for inv := range invCh {
		received = append(received, inv)
	}

	if len(received) == 0 {
		return fmt.Errorf("[P%d] Recuperación fallida: sin respuestas", p.ProcessID)
	}

	// Buscar el inventario con mayor cantidad de réplicas iguales
	chosen, count := majorityInventory(received)
	threshold := float64(len(received)) * 2.0 / 3.0
	if float64(count) < threshold {
		return fmt.Errorf("[P%d] Recuperación fallida: quórum insuficiente (%d/%d)", p.ProcessID, count, len(received))
	}

	p.st.SetInventory(chosen)
	log.Printf("[P%d] Recuperación exitosa con %d/%d votos", p.ProcessID, count, len(received))
	return nil
}

// majorityInventory devuelve el inventario más repetido y su frecuencia.
// Entrada: slice de inventarios. Salida: inventario ganador, frecuencia.
func majorityInventory(inventories [][]protocol.Item) ([]protocol.Item, int) {
	counts := make(map[string]int)
	reps := make(map[string][]protocol.Item)
	for _, inv := range inventories {
		key := inventoryKey(inv)
		counts[key]++
		reps[key] = inv
	}
	var bestKey string
	best := 0
	for k, c := range counts {
		if c > best {
			best = c
			bestKey = k
		}
	}
	return reps[bestKey], best
}

// inventoryKey serializa un inventario a string para comparación.
// Entrada: slice de Items. Salida: string clave.
func inventoryKey(inv []protocol.Item) string {
	data, _ := json.Marshal(inv)
	return string(data)
}

// appendLog escribe una línea en el log de instrucciones del proceso.
// Entrada: instrucción original, resultado. Salida: ninguna.
func (p *Process) appendLog(instruction, result string) {
	os.MkdirAll("logs", 0755)
	f, err := os.OpenFile(p.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	line := instruction
	if result != "" {
		line += " | " + result
	}
	fmt.Fprintln(f, line)
}

// writeVetoLog reescribe el archivo de vetos con el estado actual.
// Entrada: ninguna. Salida: ninguna.
func (p *Process) writeVetoLog() {
	os.MkdirAll("logs", 0755)
	vetos := p.st.GetVetos()
	f, err := os.Create(p.vetoLogPath)
	if err != nil {
		return
	}
	defer f.Close()
	for _, v := range vetos {
		fmt.Fprintf(f, "VETADO %s %d\n", v.Persona, v.Counter)
	}
}

// Status imprime el inventario y vetos actuales por stdout.
// Entrada: ninguna. Salida: ninguna (escribe a stdout).
func (p *Process) Status() {
	inv := p.st.GetInventory()
	vetos := p.st.GetVetos()
	fmt.Printf("=== Estado M%dP%d ===\n", p.MachineID, p.ProcessID)
	fmt.Println("Inventario:")
	for _, item := range inv {
		fmt.Printf("  %s: %d\n", item.Nombre, item.Cantidad)
	}
	fmt.Println("Vetos:")
	for _, v := range vetos {
		fmt.Printf("  %s (counter=%d)\n", v.Persona, v.Counter)
	}
}

// SetMalicious activa o desactiva el modo malicioso del proceso.
// Entrada: bool. Salida: ninguna.
func (p *Process) SetMalicious(m bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.malicious = m
	log.Printf("[P%d] Modo malicioso: %v", p.ProcessID, m)
}

// IsMalicious retorna si el proceso está en modo malicioso.
// Entrada: ninguna. Salida: bool.
func (p *Process) IsMalicious() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.malicious
}
