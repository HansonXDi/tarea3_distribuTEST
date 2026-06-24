// Punto de entrada del proceso expendedora.
// Cada instancia de este binario representa una máquina expendedora en el sistema distribuido.
// Los procesos se comunican entre sí via gRPC sin coordinador central.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"tarea3/internal/estado"
	"tarea3/internal/grpcserver"
	"tarea3/internal/instrucciones"
	"tarea3/internal/logutil"
	"tarea3/internal/pares"
)

func main() {
	// --- Argumentos de línea de comandos ---
	maquina := flag.Int("maquina", 0, "Número de máquina virtual (1, 2 o 3)")
	proceso := flag.Int("proceso", 0, "ID del proceso (número del archivo instrucciones)")
	restaurar := flag.Bool("restaurar", false, "Indica si este proceso se está restaurando")
	flag.Parse()

	if *maquina == 0 || *proceso == 0 {
		fmt.Fprintln(os.Stderr, "Uso: expendedora -maquina <N> -proceso <ID> [-restaurar]")
		os.Exit(1)
	}

	rand.Seed(time.Now().UnixNano() + int64(*proceso))

	// --- Inicializar logger de proceso ---
	logger := logutil.NuevoLogger(*maquina, *proceso)
	logger.Infof("Iniciando expendedora M%dP%d", *maquina, *proceso)

	// --- Cargar archivo de instrucciones ---
	archivoInstr, err := buscarArchivoInstruccion(*proceso)
	if err != nil {
		logger.Errorf("No se encontró archivo de instrucciones para proceso %d: %v", *proceso, err)
		os.Exit(1)
	}
	logger.Infof("Usando instrucciones: %s", archivoInstr)

	// --- Cargar inventario inicial (solo si no es restauración) ---
	var inventarioInicial []estado.Item
	if !*restaurar {
		inventarioInicial, err = cargarInventarioAleatorio()
		if err != nil {
			logger.Errorf("No se pudo cargar inventario: %v", err)
			os.Exit(1)
		}
		logger.Infof("Inventario inicial cargado con %d productos", len(inventarioInicial))
	}

	// --- Crear estado del proceso ---
	proc := estado.NuevoProceso(*maquina, *proceso, inventarioInicial)

	// --- Iniciar servidor gRPC ---
	puerto := calcularPuerto(*maquina, *proceso)
	srv, err := grpcserver.Iniciar(proc, puerto, logger)
	if err != nil {
		logger.Errorf("No se pudo iniciar servidor gRPC en puerto %d: %v", puerto, err)
		os.Exit(1)
	}
	logger.Infof("Servidor gRPC escuchando en puerto %d", puerto)

	// --- Manejador de señal SIGUSR1: toggle infección ---
	// El script bash envía SIGUSR1 al ejecutar INFECTAR
	canalSignal := make(chan os.Signal, 1)
	signal.Notify(canalSignal, syscall.SIGUSR1)
	go func() {
		for range canalSignal {
			infectado := proc.ToggleInfeccion()
			if infectado {
				logger.Infof("*** MODO INFECCIÓN ACTIVADO: enviará inventarios alterados ***")
			} else {
				logger.Infof("*** MODO INFECCIÓN DESACTIVADO: volviendo a comportamiento normal ***")
			}
		}
	}()

	// --- Esperar a que todos los procesos del sistema estén listos ---
	gestorPares := pares.NuevoGestor(*maquina, *proceso, proc, logger)
	if err := gestorPares.EsperarSistema(2 * time.Second); err != nil {
		logger.Errorf("Error esperando inicialización del sistema: %v", err)
		os.Exit(1)
	}
	logger.Infof("Sistema inicializado. Pares conocidos: %d", gestorPares.CantidadPares())

	// --- Recuperación de estado (si aplica) ---
	if *restaurar {
		logger.Infof("Iniciando recuperación de estado...")
		if err := gestorPares.RecuperarEstado(proc); err != nil {
			logger.Errorf("ERROR CRITICO: No se pudo recuperar estado: %v", err)
			fmt.Fprintf(os.Stderr,
				"[ERROR CRITICO] M%dP%d: Quorum no alcanzado. El proceso no puede unirse al sistema.\n"+
					"Detalle: %v\n", *maquina, *proceso, err)
			srv.Detener()
			os.Exit(2)
		}
		logger.Infof("Estado recuperado exitosamente")
	} else {
		// Si es inicio fresco, replicar inventario inicial a todos los pares
		gestorPares.ReplicarInventario(proc.ObtenerInventario())
	}

	// --- Ejecutar instrucciones de forma concurrente con recepción de mensajes ---
	// El servidor gRPC ya corre en su goroutine; el ejecutor corre en otra.
	ejecutor := instrucciones.NuevoEjecutor(proc, gestorPares, logger)
	go ejecutor.EjecutarArchivo(archivoInstr)

	// --- Mantener proceso vivo ---
	// Los manejadores SIGTERM/SIGINT permiten cierre elegante.
	logger.Infof("Expendedora M%dP%d lista y escuchando", *maquina, *proceso)
	canalTerminar := make(chan os.Signal, 1)
	signal.Notify(canalTerminar, syscall.SIGTERM, syscall.SIGINT)
	<-canalTerminar

	logger.Infof("Señal de terminación recibida. Cerrando M%dP%d...", *maquina, *proceso)
	logger.Cerrar()
	srv.Detener()
}

// buscarArchivoInstruccion busca en la carpeta "instrucciones" el archivo cuyo nombre
// termina en _<id>.txt (formato: <nombre_cualquiera>_ID.txt).
func buscarArchivoInstruccion(id int) (string, error) {
	sufijo := fmt.Sprintf("_%d.txt", id)
	entries, err := os.ReadDir("instrucciones")
	if err != nil {
		return "", fmt.Errorf("no se pudo leer carpeta instrucciones: %w", err)
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), sufijo) {
			return filepath.Join("instrucciones", e.Name()), nil
		}
	}
	return "", fmt.Errorf("no hay archivo instrucciones con id %d (sufijo %s)", id, sufijo)
}

// cargarInventarioAleatorio elige un archivo JSON aleatorio de la carpeta "inventario"
// y carga su contenido como lista de items.
func cargarInventarioAleatorio() ([]estado.Item, error) {
	entries, err := os.ReadDir("inventario")
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer carpeta inventario: %w", err)
	}
	var jsons []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			jsons = append(jsons, filepath.Join("inventario", e.Name()))
		}
	}
	if len(jsons) == 0 {
		return nil, fmt.Errorf("no hay archivos JSON en carpeta inventario")
	}
	elegido := jsons[rand.Intn(len(jsons))]
	data, err := os.ReadFile(elegido)
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer %s: %w", elegido, err)
	}
	var items []estado.Item
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("JSON inválido en %s: %w", elegido, err)
	}
	log.Printf("[inventario] Cargado desde %s", elegido)
	return items, nil
}

// calcularPuerto asigna un puerto único a cada proceso basándose en máquina e ID.
// Fórmula: 50000 + (maquina-1)*100 + proceso
// Ejemplos: M1P1=50001, M1P2=50002, M2P1=50101, M3P5=50205
func calcularPuerto(maquina, proceso int) int {
	return 50000 + (maquina-1)*100 + proceso
}
