package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"tarea3/internal/process"
	netsync "tarea3/internal/sync"
)

// Mapa fijo de IPs por máquina.
var machineIPs = map[int]string{
	1: "10.10.28.35",
	2: "10.10.28.36",
	3: "10.10.28.37",
}

// basePort calcula el puerto TCP de un proceso a partir de su máquina y ID.
// Entrada: machineID, processID. Salida: int puerto (ej: M1P1 -> 8001).
func basePort(machineID, processID int) int {
	return 8000 + (machineID-1)*100 + processID
}

// buildPeers construye la lista de todos los peers del sistema excepto el proceso actual.
// Entrada: myMachine, myProcess, numProcs por máquina. Salida: slice de PeerConfig.
func buildPeers(myMachine, myProcess, numProcs int) []netsync.PeerConfig {
	var peers []netsync.PeerConfig
	for mID, ip := range machineIPs {
		for pID := 1; pID <= numProcs; pID++ {
			if mID == myMachine && pID == myProcess {
				continue
			}
			peers = append(peers, netsync.PeerConfig{
				MachineID: mID,
				ProcessID: pID,
				Addr:      fmt.Sprintf("%s:%d", ip, basePort(mID, pID)),
			})
		}
	}
	return peers
}

// findInstructionFile busca en instrucciones/ el archivo terminado en _<ID>.txt.
// Entrada: processID. Salida: path del archivo o "" si no existe.
func findInstructionFile(processID int) string {
	suffix := fmt.Sprintf("_%d.txt", processID)
	matches, _ := filepath.Glob("instrucciones/*.txt")
	for _, m := range matches {
		if strings.HasSuffix(m, suffix) {
			return m
		}
	}
	fallback := fmt.Sprintf("instrucciones/proceso_%d.txt", processID)
	if _, err := os.Stat(fallback); err == nil {
		return fallback
	}
	return ""
}

// runSingleProcess ejecuta UN proceso lógico como proceso real del sistema operativo.
// Maneja SIGUSR1 (toggle modo malicioso) y SIGTERM/SIGINT (apagado limpio).
// Entrada: machineID, processID, numProcs (para calcular peers). Salida: ninguna (bloquea).
func runSingleProcess(machineID, processID, numProcs int) {
	rand.Seed(time.Now().UnixNano() + int64(processID))

	instrFile := findInstructionFile(processID)
	if instrFile == "" {
		log.Printf("Advertencia: sin archivo de instrucciones para proceso %d", processID)
		instrFile = fmt.Sprintf("instrucciones/proceso_%d.txt", processID)
	}
	peers := buildPeers(machineID, processID, numProcs)
	port := basePort(machineID, processID)
	p := process.New(machineID, processID, instrFile, peers, port)

	// Escuchar SIGUSR1 para toggle de modo malicioso de ESTE proceso individual
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGUSR1)
	go func() {
		for range sigCh {
			p.SetMalicious(!p.IsMalicious())
		}
	}()

	p.Start()
	select {}
}

// recoverProcess restaura un proceso específico mediante el protocolo de quórum de 2/3.
// Entrada: machineID, processID, numProcs por máquina. Salida: ninguna (falla con log.Fatal si no hay quórum).
func recoverProcess(machineID, processID, numProcs int) {
	rand.Seed(time.Now().UnixNano() + int64(processID))

	instrFile := findInstructionFile(processID)
	if instrFile == "" {
		instrFile = fmt.Sprintf("instrucciones/proceso_%d.txt", processID)
	}
	peers := buildPeers(machineID, processID, numProcs)
	port := basePort(machineID, processID)
	p := process.New(machineID, processID, instrFile, peers, port)

	if err := p.Recover(); err != nil {
		log.Fatalf("Recuperación fallida: %v", err)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGUSR1)
	go func() {
		for range sigCh {
			p.SetMalicious(!p.IsMalicious())
		}
	}()

	p.Start()
	select {}
}

// printStatusFromLogs muestra el inventario y vetos actuales de un proceso leyendo
// directamente los archivos que el proceso real escribe en disco (logs/snapshot_*.json
// y logs/vetos_*.log), sin necesitar crear una instancia nueva que choque con el
// puerto TCP ya ocupado por el proceso en ejecución.
// Entrada: machineID, processID. Salida: ninguna (imprime a stdout).
func printStatusFromLogs(machineID, processID int) {
	fmt.Printf("=== Estado M%dP%d ===\n", machineID, processID)

	snapPath := fmt.Sprintf("logs/snapshot_M%dP%d.json", machineID, processID)
	fmt.Println("Inventario:")
	if data, err := os.ReadFile(snapPath); err == nil {
		var items []struct {
			Nombre   string `json:"nombre"`
			Cantidad int    `json:"cantidad"`
		}
		if json.Unmarshal(data, &items) == nil {
			for _, it := range items {
				fmt.Printf("  %s: %d\n", it.Nombre, it.Cantidad)
			}
		}
	} else {
		fmt.Println("  (sin datos; el proceso aún no ha escrito snapshot)")
	}

	vetoPath := fmt.Sprintf("logs/vetos_M%dP%d.log", machineID, processID)
	fmt.Println("Vetos:")
	if data, err := os.ReadFile(vetoPath); err == nil {
		content := strings.TrimSpace(string(data))
		if content == "" {
			fmt.Println("  (ninguno)")
		} else {
			fmt.Println(content)
		}
	} else {
		fmt.Println("  (sin datos)")
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Uso: ./expendedora <MAQUINA> <N|RESTAURAR|ESTADO> [ID]")
		os.Exit(1)
	}

	machineID, err := strconv.Atoi(os.Args[1])
	if err != nil {
		log.Fatalf("NUMERO_DE_MAQUINA inválido: %s", os.Args[1])
	}

	if len(os.Args) < 3 {
		fmt.Println("Falta segundo argumento.")
		os.Exit(1)
	}

	switch strings.ToUpper(os.Args[2]) {
	case "RESTAURAR":
		if len(os.Args) < 4 {
			log.Fatal("Uso: ./expendedora <MAQUINA> RESTAURAR <ID> [NUM_PROCESOS]")
		}
		id, _ := strconv.Atoi(os.Args[3])
		numProcs := 2
		if len(os.Args) >= 5 {
			numProcs, _ = strconv.Atoi(os.Args[4])
		}
		recoverProcess(machineID, id, numProcs)

	case "ESTADO":
		if len(os.Args) < 4 {
			log.Fatal("Uso: ./expendedora <MAQUINA> ESTADO <ID>")
		}
		id, _ := strconv.Atoi(os.Args[3])
		printStatusFromLogs(machineID, id)

	case "PROC":
		// Modo interno usado por script.sh: ejecuta UN proceso lógico como
		// proceso real del SO. Uso: ./expendedora <MAQUINA> PROC <ID> <NUM_PROCESOS_TOTAL>
		if len(os.Args) < 5 {
			log.Fatal("Uso: ./expendedora <MAQUINA> PROC <ID> <NUM_PROCESOS_TOTAL>")
		}
		id, _ := strconv.Atoi(os.Args[3])
		numProcs, _ := strconv.Atoi(os.Args[4])
		runSingleProcess(machineID, id, numProcs)

	default:
		log.Fatalf("Argumento inválido: %s (use RESTAURAR, ESTADO o PROC)", os.Args[2])
	}
}
