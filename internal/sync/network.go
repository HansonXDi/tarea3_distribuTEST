package sync

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"
	"tarea3/internal/protocol"
	"time"
)

// PeerConfig almacena la dirección de un peer remoto.
type PeerConfig struct {
	MachineID int
	ProcessID int
	Addr      string
}

// Network gestiona las conexiones TCP con los peers y el servidor de escucha local.
type Network struct {
	mu          sync.RWMutex
	machineID   int
	processID   int
	listenPort  int
	peers       []PeerConfig
	conns       map[string]net.Conn // addr -> conn
	malicious   *bool               // puntero compartido con el proceso padre
	RecvCh      chan protocol.Message
}

// NewNetwork crea la capa de red para un proceso.
// Entrada: machineID, processID, puerto de escucha, peers, puntero a modo malicioso.
// Salida: *Network listo para usar.
func NewNetwork(machineID, processID, port int, peers []PeerConfig, malicious *bool) *Network {
	return &Network{
		machineID:  machineID,
		processID:  processID,
		listenPort: port,
		peers:      peers,
		conns:      make(map[string]net.Conn),
		malicious:  malicious,
		RecvCh:     make(chan protocol.Message, 256),
	}
}

// Listen inicia el servidor TCP y acepta conexiones entrantes en una goroutine.
// Entrada: ninguna. Salida: error si no puede abrir el puerto.
func (n *Network) Listen() error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", n.listenPort))
	if err != nil {
		return err
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				continue
			}
			go n.handleConn(conn)
		}
	}()
	return nil
}

// handleConn decodifica mensajes JSON de una conexión entrante y los envía al canal.
// Entrada: net.Conn. Salida: ninguna (loop hasta cierre).
func (n *Network) handleConn(conn net.Conn) {
	defer conn.Close()
	dec := json.NewDecoder(conn)
	for {
		var msg protocol.Message
		if err := dec.Decode(&msg); err != nil {
			return
		}
		n.RecvCh <- msg
	}
}

// ConnectPeers intenta conectarse a todos los peers con reintentos.
// Entrada: ninguna. Salida: ninguna (bloquea hasta conectar o agotar reintentos).
func (n *Network) ConnectPeers() {
	for _, p := range n.peers {
		go n.connectPeer(p)
	}
}

// connectPeer intenta conectarse a un peer específico, reintentando hasta 30 veces.
// Entrada: PeerConfig. Salida: ninguna.
func (n *Network) connectPeer(p PeerConfig) {
	for i := 0; i < 30; i++ {
		conn, err := net.DialTimeout("tcp", p.Addr, 2*time.Second)
		if err == nil {
			n.mu.Lock()
			n.conns[p.Addr] = conn
			n.mu.Unlock()
			log.Printf("[NET] Conectado a %s", p.Addr)
			// Enviar HELLO
			hello := protocol.Message{
				Type:      protocol.MsgHello,
				MachineID: n.machineID,
				ProcessID: n.processID,
			}
			_ = n.sendTo(conn, hello)
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	log.Printf("[NET] No se pudo conectar a %s", p.Addr)
}

// Broadcast envía un mensaje a todos los peers conectados.
// Entrada: Message. Salida: ninguna.
func (n *Network) Broadcast(msg protocol.Message) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	for addr, conn := range n.conns {
		if err := n.sendTo(conn, msg); err != nil {
			log.Printf("[NET] Error enviando a %s: %v", addr, err)
		}
	}
}

// BroadcastInventory envía el inventario a todos los peers; si está en modo malicioso, lo corrompe.
// Entrada: inventario actual. Salida: ninguna.
func (n *Network) BroadcastInventory(inv []protocol.Item) {
	msg := protocol.Message{
		Type:      protocol.MsgInventory,
		MachineID: n.machineID,
		ProcessID: n.processID,
		Inventory: inv,
	}
	if n.malicious != nil && *n.malicious {
		// Corrompe el inventario antes de enviarlo
		msg.Type = protocol.MsgMalicious
		for i := range msg.Inventory {
			msg.Inventory[i].Cantidad = -999
		}
	}
	n.Broadcast(msg)
}

// sendTo serializa y envía un mensaje JSON por una conexión TCP.
// Entrada: net.Conn, Message. Salida: error si falla la escritura.
func (n *Network) sendTo(conn net.Conn, msg protocol.Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = conn.Write(data)
	return err
}

// SendTo envía un mensaje a un peer específico por dirección.
// Entrada: addr string, Message. Salida: error si no hay conexión o falla el envío.
func (n *Network) SendTo(addr string, msg protocol.Message) error {
	n.mu.RLock()
	conn, ok := n.conns[addr]
	n.mu.RUnlock()
	if !ok {
		return fmt.Errorf("sin conexión a %s", addr)
	}
	return n.sendTo(conn, msg)
}

// ConnectedCount retorna la cantidad de peers actualmente conectados.
// Entrada: ninguna. Salida: int.
func (n *Network) ConnectedCount() int {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return len(n.conns)
}
