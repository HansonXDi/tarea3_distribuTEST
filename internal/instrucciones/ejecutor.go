// Package instrucciones implementa la lectura y ejecución de archivos de instrucciones.
// Cada proceso lee su archivo proceso_ID.txt y ejecuta cada instrucción de forma secuencial
// (VETAR, COMPRAR, PERDONAR), decrement el counter de vetos después de cada instrucción,
// y replica los cambios a los demás procesos del sistema.
package instrucciones

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"tarea3/internal/estado"
	"tarea3/internal/logutil"
	"tarea3/internal/pares"
)

// Ejecutor maneja la ejecución del archivo de instrucciones de un proceso.
type Ejecutor struct {
	proc   *estado.Proceso
	gestor *pares.Gestor
	logger *logutil.Logger
}

// NuevoEjecutor crea un ejecutor para el proceso dado.
func NuevoEjecutor(proc *estado.Proceso, gestor *pares.Gestor, logger *logutil.Logger) *Ejecutor {
	return &Ejecutor{proc: proc, gestor: gestor, logger: logger}
}

// EjecutarArchivo lee el archivo de instrucciones línea por línea y ejecuta cada una.
// Las líneas vacías y comentarios (iniciados con #) son ignorados.
// Cada instrucción decrementa el counter de vetos activos al finalizar.
func (e *Ejecutor) EjecutarArchivo(ruta string) {
	f, err := os.Open(ruta)
	if err != nil {
		e.logger.Errorf("No se pudo abrir archivo de instrucciones %s: %v", ruta, err)
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	numLinea := 0
	for scanner.Scan() {
		linea := strings.TrimSpace(scanner.Text())
		numLinea++

		if linea == "" || strings.HasPrefix(linea, "#") {
			continue
		}

		e.logger.Infof("Ejecutando instrucción %d: %s", numLinea, linea)
		e.ejecutarInstruccion(linea)

		// Después de cada instrucción, decrementar counters de vetos
		// y perdonar automáticamente a quienes lleguen a 0
		perdonados := e.proc.DecrementarCounterVetos()
		for _, persona := range perdonados {
			e.logger.RegistrarInstruccion(fmt.Sprintf("PERDONAR %s", persona), "(auto)")
			e.logger.Infof("Veto de '%s' expiró (counter=0), perdonado automáticamente", persona)
		}

		// Si hubo perdonados automáticos, replicar vetos
		if len(perdonados) > 0 {
			vetos := e.proc.ObtenerVetos()
			e.logger.ActualizarLogVetos(vetos)
			go e.gestor.ReplicarVetos(vetos)
		}
	}

	if err := scanner.Err(); err != nil {
		e.logger.Errorf("Error leyendo instrucciones: %v", err)
	}
	e.logger.Infof("Archivo de instrucciones completado: %s", ruta)
}

// ejecutarInstruccion parsea y ejecuta una línea del archivo de instrucciones.
func (e *Ejecutor) ejecutarInstruccion(linea string) {
	partes := strings.Fields(linea)
	if len(partes) == 0 {
		return
	}

	cmd := strings.ToUpper(partes[0])
	switch cmd {
	case "VETAR":
		e.ejecutarVetar(partes[1:], linea)
	case "COMPRAR":
		e.ejecutarComprar(partes[1:], linea)
	case "PERDONAR":
		e.ejecutarPerdonar(partes[1:], linea)
	default:
		e.logger.Errorf("Instrucción desconocida: %s", cmd)
	}
}

// ejecutarVetar implementa la instrucción VETAR <nombre_persona>.
// El nombre puede ser compuesto (varias palabras).
// Añade o renueva el veto con counter=5 y replica a todos los pares.
func (e *Ejecutor) ejecutarVetar(args []string, lineaOriginal string) {
	if len(args) == 0 {
		e.logger.Errorf("VETAR requiere un nombre de persona")
		return
	}
	persona := strings.Join(args, " ")
	esNuevo := e.proc.VetarPersona(persona)

	if esNuevo {
		e.logger.Infof("Persona '%s' vetada (nuevo)", persona)
	} else {
		e.logger.Infof("Veto de '%s' renovado (counter=5)", persona)
	}

	// Registrar en log de instrucciones
	e.logger.RegistrarInstruccion(lineaOriginal)

	// Actualizar log de vetos y replicar
	vetos := e.proc.ObtenerVetos()
	e.logger.ActualizarLogVetos(vetos)
	go e.gestor.ReplicarVetos(vetos)
}

// ejecutarComprar implementa la instrucción COMPRAR <persona> <producto> <cantidad>.
// El nombre de la persona puede ser compuesto. El producto es una sola palabra.
// La cantidad es el último token.
// Verifica veto, verifica stock, y si es válido descuenta del inventario y replica.
func (e *Ejecutor) ejecutarComprar(args []string, lineaOriginal string) {
	// COMPRAR <persona...> <producto> <cantidad>
	// La persona puede tener múltiples palabras; el producto es penúltimo, cantidad el último.
	if len(args) < 3 {
		e.logger.Errorf("COMPRAR requiere: persona producto cantidad")
		return
	}

	// Parsear: último token es cantidad, penúltimo es producto, el resto es persona
	cantidadStr := args[len(args)-1]
	producto := args[len(args)-2]
	persona := strings.Join(args[:len(args)-2], " ")

	var cantidad int
	if _, err := fmt.Sscan(cantidadStr, &cantidad); err != nil || cantidad <= 0 {
		e.logger.Errorf("Cantidad inválida en COMPRAR: %s", cantidadStr)
		e.logger.RegistrarInstruccion(lineaOriginal, "NO VALIDO")
		return
	}

	resultado := e.proc.IntentarCompra(persona, producto, cantidad)
	e.logger.RegistrarInstruccion(lineaOriginal, resultado)
	e.logger.Infof("COMPRAR %s %s %d -> %s", persona, producto, cantidad, resultado)

	// Si la compra fue exitosa, replicar inventario actualizado
	if resultado == "VALIDO" {
		inventario := e.proc.ObtenerInventario()
		go e.gestor.ReplicarInventario(inventario)
	}
}

// ejecutarPerdonar implementa la instrucción PERDONAR <nombre_persona>.
// Elimina el veto de la persona y replica la lista actualizada a todos los pares.
func (e *Ejecutor) ejecutarPerdonar(args []string, lineaOriginal string) {
	if len(args) == 0 {
		e.logger.Errorf("PERDONAR requiere un nombre de persona")
		return
	}
	persona := strings.Join(args, " ")
	estaba := e.proc.PerdonarPersona(persona)

	if estaba {
		e.logger.Infof("Persona '%s' perdonada", persona)
	} else {
		e.logger.Infof("PERDONAR '%s': la persona no estaba vetada (ignorado)", persona)
	}

	e.logger.RegistrarInstruccion(lineaOriginal)

	// Actualizar log de vetos y replicar
	vetos := e.proc.ObtenerVetos()
	e.logger.ActualizarLogVetos(vetos)
	go e.gestor.ReplicarVetos(vetos)
}
