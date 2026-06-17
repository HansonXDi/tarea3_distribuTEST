package main

import (
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

// startProcesses lanza numProcs procesos en la máquina indicada y maneja SIGUSR1 para modo malicioso.
// Entrada: machineID, numProcs. Salida: ninguna (bloquea indefinidamente).
func startProcesses(machineID, numProcs int) {
	rand.Seed(time.Now().UnixNano())
	var procs []*process.Process

	for pID := 1; pID <= numProcs; pID++ {
		instrFile := findInstructionFile(pID)
		if instrFile == "" {
			log.Printf("Advertencia: sin archivo de instrucciones para proceso %d", pID)
			instrFile = fmt.Sprintf("instrucciones/proceso_%d.txt", pID)
		}
		peers := buildPeers(machineID, pID, numProcs)
		port := basePort(machineID, pID)
		p := process.New(machineID, pID, instrFile, peers, port)
		procs = append(procs, p)
	}

	for _, p := range procs {
		go p.Start()
	}

	// Escuchar SIGUSR1 para toggle de modo malicioso
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGUSR1)
	go func() {
		for range sigCh {
			for _, p := range procs {
				p.SetMalicious(!p.IsMalicious())
			}
		}
	}()

	select {}
}

// recoverProcess restaura un proceso específico mediante el protocolo de quórum de 2/3.
// Entrada: machineID, processID. Salida: ninguna (falla con log.Fatal si no hay quórum).
func recoverProcess(machineID, processID int) {
	instrFile := findInstructionFile(processID)
	if instrFile == "" {
		instrFile = fmt.Sprintf("instrucciones/proceso_%d.txt", processID)
	}
	peers := buildPeers(machineID, processID, 3)
	port := basePort(machineID, processID)
	p := process.New(machineID, processID, instrFile, peers, port)

	if err := p.Recover(); err != nil {
		log.Fatalf("Recuperación fallida: %v", err)
	}
	go p.Start()
	select {}
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
			log.Fatal("Uso: ./expendedora <MAQUINA> RESTAURAR <ID>")
		}
		id, _ := strconv.Atoi(os.Args[3])
		recoverProcess(machineID, id)

	case "ESTADO":
		if len(os.Args) < 4 {
			log.Fatal("Uso: ./expendedora <MAQUINA> ESTADO <ID>")
		}
		id, _ := strconv.Atoi(os.Args[3])
		instrFile := findInstructionFile(id)
		peers := buildPeers(machineID, id, 3)
		port := basePort(machineID, id)
		p := process.New(machineID, id, instrFile, peers, port)
		p.Status()

	default:
		numProcs, err := strconv.Atoi(os.Args[2])
		if err != nil {
			log.Fatalf("Argumento inválido: %s", os.Args[2])
		}
		startProcesses(machineID, numProcs)
	}
}
