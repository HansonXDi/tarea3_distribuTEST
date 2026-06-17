package protocol

// MsgType identifica el tipo de mensaje entre procesos.
type MsgType string

const (
	MsgHello        MsgType = "HELLO"
	MsgInventory    MsgType = "INVENTORY"
	MsgVeto         MsgType = "VETO"
	MsgPardon       MsgType = "PARDON"
	MsgSyncVetos    MsgType = "SYNC_VETOS"
	MsgRecover      MsgType = "RECOVER"
	MsgRecoverReply MsgType = "RECOVER_REPLY"
	MsgMalicious    MsgType = "MALICIOUS_INV"
)

// Item representa un producto en el inventario.
type Item struct {
	Nombre   string `json:"nombre"`
	Cantidad int    `json:"cantidad"`
}

// VetoEntry representa un veto activo con su counter.
type VetoEntry struct {
	Persona string `json:"persona"`
	Counter int    `json:"counter"`
}

// Message es el sobre genérico que viaja por la red.
type Message struct {
	Type      MsgType     `json:"type"`
	MachineID int         `json:"machine_id"`
	ProcessID int         `json:"process_id"`
	Inventory []Item      `json:"inventory,omitempty"`
	Vetos     []VetoEntry `json:"vetos,omitempty"`
	VetoName  string      `json:"veto_name,omitempty"`
	Counter   int         `json:"counter,omitempty"`
}
