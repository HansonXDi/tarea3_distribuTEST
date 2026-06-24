// Package logutil provee un logger estructurado para cada proceso expendedora.
// Genera logs en consola con prefijo [M<m>P<p>] y además escribe los archivos
// de log de inventario y vetos en la carpeta logs/, replicándolos a los otros nodos.
package logutil

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
)

// Logger es el logger de un proceso específico.
type Logger struct {
	maquina  int
	proceso  int
	prefijo  string
	muLog    sync.Mutex
	logInv   *os.File // archivo logs/inventario_M<m>P<p>.log
	logVetos *os.File // archivo logs/vetos_M<m>P<p>.log
}

// NuevoLogger crea un logger para el proceso dado y abre/crea los archivos de log.
func NuevoLogger(maquina, proceso int) *Logger {
	if err := os.MkdirAll("logs", 0755); err != nil {
		log.Fatalf("No se pudo crear carpeta logs: %v", err)
	}

	archivoInv := filepath.Join("logs", fmt.Sprintf("inventario_M%dP%d.log", maquina, proceso))
	archivoVetos := filepath.Join("logs", fmt.Sprintf("vetos_M%dP%d.log", maquina, proceso))

	fInv, err := os.OpenFile(archivoInv, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		log.Fatalf("No se pudo abrir log de inventario: %v", err)
	}

	fVetos, err := os.OpenFile(archivoVetos, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		log.Fatalf("No se pudo abrir log de vetos: %v", err)
	}

	return &Logger{
		maquina:  maquina,
		proceso:  proceso,
		prefijo:  fmt.Sprintf("[M%dP%d]", maquina, proceso),
		logInv:   fInv,
		logVetos: fVetos,
	}
}

// Infof escribe un mensaje informativo en stdout con prefijo del proceso.
func (l *Logger) Infof(formato string, args ...interface{}) {
	log.Printf(l.prefijo+" "+formato, args...)
}

// Errorf escribe un mensaje de error en stderr con prefijo del proceso.
func (l *Logger) Errorf(formato string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, l.prefijo+" [ERROR] "+formato+"\n", args...)
}

// RegistrarInstruccion escribe una línea en el log de inventario con el resultado
// de ejecutar una instrucción (VETAR, COMPRAR, PERDONAR).
// Formato: INSTRUCCION args | RESULTADO  (o solo INSTRUCCION si no hay resultado)
func (l *Logger) RegistrarInstruccion(instruccion string, resultado ...string) {
	l.muLog.Lock()
	defer l.muLog.Unlock()

	var linea string
	if len(resultado) > 0 && resultado[0] != "" {
		linea = fmt.Sprintf("%s | %s\n", instruccion, resultado[0])
	} else {
		linea = instruccion + "\n"
	}

	if _, err := l.logInv.WriteString(linea); err != nil {
		fmt.Fprintf(os.Stderr, "%s Error escribiendo log inventario: %v\n", l.prefijo, err)
	}

	// También imprimir en stdout para visibilidad
	log.Printf(l.prefijo+" "+instruccion+
		func() string {
			if len(resultado) > 0 && resultado[0] != "" {
				return " -> " + resultado[0]
			}
			return ""
		}())
}

// ActualizarLogVetos reescribe completamente el archivo de log de vetos
// con el estado actual de la lista. Formato: VETADO <persona> <counter>
func (l *Logger) ActualizarLogVetos(vetos map[string]int) {
	l.muLog.Lock()
	defer l.muLog.Unlock()

	// Truncar y reescribir
	if err := l.logVetos.Truncate(0); err != nil {
		fmt.Fprintf(os.Stderr, "%s Error truncando log vetos: %v\n", l.prefijo, err)
		return
	}
	if _, err := l.logVetos.Seek(0, 0); err != nil {
		return
	}

	for persona, counter := range vetos {
		linea := fmt.Sprintf("VETADO %s %d\n", persona, counter)
		if _, err := l.logVetos.WriteString(linea); err != nil {
			fmt.Fprintf(os.Stderr, "%s Error escribiendo log vetos: %v\n", l.prefijo, err)
			return
		}
	}
}

// Cerrar cierra los archivos de log abiertos.
func (l *Logger) Cerrar() {
	_ = l.logInv.Close()
	_ = l.logVetos.Close()
}

// RutaLogInventario retorna la ruta del archivo de log de inventario de este proceso.
func (l *Logger) RutaLogInventario() string {
	return filepath.Join("logs", fmt.Sprintf("inventario_M%dP%d.log", l.maquina, l.proceso))
}

// RutaLogVetos retorna la ruta del archivo de log de vetos de este proceso.
func (l *Logger) RutaLogVetos() string {
	return filepath.Join("logs", fmt.Sprintf("vetos_M%dP%d.log", l.maquina, l.proceso))
}
