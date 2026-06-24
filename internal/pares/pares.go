// Package pares gestiona el conjunto de pares (otros procesos) conocidos por esta expendedora.
// Se encarga de:
//   - Descubrimiento inicial: anunciar la propia existencia y esperar a los demás
//   - Replicación: enviar actualizaciones de inventario y vetos a todos los pares
//   - Recuperación: solicitar estado a todos los pares y aplicar quorum 2/3
//
// No existe coordinador central; cada proceso habla directamente con los demás vía gRPC.
package pares

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"google.golang.org/grpc"

	"tarea3/internal/estado"
	"tarea3/internal/grpcserver"
	"tarea3/internal/logutil"
	pb "tarea3/proto/expendedorapb"
)

// IPs fijas de las tres máquinas virtuales del sistema.
var ipsMaquinas = map[int]string{
	1: "10.10.28.35",
	2: "10.10.28.36",
	3: "10.10.28.37",
}

// archivoTopologia es el nombre del archivo que persiste la topología conocida.
const archivoTopologia = "topologia.json"

// infopar contiene la dirección y cliente gRPC de un par conocido.
type infopar struct {
	Maquina   int
	Proceso   int
	Direccion string
	Cliente   pb.ExpendedoraClient
	conn      *grpc.ClientConn
}

// respuestaPar agrupa el inventario y vetos recibidos de un par durante recuperación.
type respuestaPar struct {
	items []estado.Item
	vetos map[string]int
}

// Gestor administra la lista de pares y provee operaciones de comunicación grupales.
type Gestor struct {
	mu      sync.RWMutex
	maquina int
	proceso int
	proc    *estado.Proceso
	logger  *logutil.Logger
	pares   map[string]*infopar // clave: "M<m>P<p>"
}

// NuevoGestor crea un gestor de pares para el proceso dado.
func NuevoGestor(maquina, proceso int, proc *estado.Proceso, logger *logutil.Logger) *Gestor {
	return &Gestor{
		maquina: maquina,
		proceso: proceso,
		proc:    proc,
		logger:  logger,
		pares:   make(map[string]*infopar),
	}
}

// EsperarSistema implementa la lógica de inicio del sistema:
//  1. Lee la topología del archivo topologia.json (escrito por el script bash).
//  2. Conecta a todos los pares conocidos.
//  3. Anuncia la propia existencia.
//  4. Espera hasta `timeout` recibiendo anuncios de nuevos pares.
func (g *Gestor) EsperarSistema(timeout time.Duration) error {
	topologia, err := cargarTopologia()
	if err != nil {
		return fmt.Errorf("no se pudo cargar topologia.json: %w", err)
	}
	g.logger.Infof("Topología cargada: %d procesos en el sistema", len(topologia))

	// Conectar a los pares ya conocidos en topología
	for _, entrada := range topologia {
		if entrada.Maquina == g.maquina && entrada.Proceso == g.proceso {
			continue // no conectar con uno mismo
		}
		g.conectarAPar(entrada.Maquina, entrada.Proceso, entrada.Direccion)
	}

	// Anunciar la propia existencia a todos los pares conectados
	miDireccion := fmt.Sprintf("%s:%d", ipsMaquinas[g.maquina], calcularPuerto(g.maquina, g.proceso))
	g.anunciarA(miDireccion)

	// Esperar anuncios de pares durante la ventana de 2 segundos
	canal := grpcserver.ObtenerServidorApp().CanalRegistro
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case reg := <-canal:
			g.conectarAPar(int(reg.Id.Maquina), int(reg.Id.Proceso), reg.Direccion)
		case <-timer.C:
			g.logger.Infof("Fase de espera completada. Pares conectados: %d", len(g.pares))
			return nil
		}
	}
}

// CantidadPares retorna el número de pares conocidos (excluyendo este proceso).
func (g *Gestor) CantidadPares() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.pares)
}

// ReplicarInventario envía el inventario actualizado a todos los pares conectados en paralelo.
func (g *Gestor) ReplicarInventario(items []estado.Item) {
	g.mu.RLock()
	pares := g.copiarPares()
	g.mu.RUnlock()

	pbItems := estadoItemsAPb(items)
	origen := &pb.IDProceso{
		Maquina: int32(g.maquina),
		Proceso: int32(g.proceso),
	}

	var wg sync.WaitGroup
	for _, par := range pares {
		wg.Add(1)
		go func(p *infopar) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_, err := p.Cliente.ActualizarInventario(ctx, &pb.ActualizacionInventario{
				Origen: origen,
				Items:  pbItems,
			})
			if err != nil {
				g.logger.Errorf("Error replicando inventario a M%dP%d: %v", p.Maquina, p.Proceso, err)
			}
		}(par)
	}
	wg.Wait()
}

// ReplicarVetos envía la lista de vetos actualizada a todos los pares conectados.
func (g *Gestor) ReplicarVetos(vetos map[string]int) {
	g.mu.RLock()
	pares := g.copiarPares()
	g.mu.RUnlock()

	pbVetos := mapaVetosAPb(vetos)
	origen := &pb.IDProceso{
		Maquina: int32(g.maquina),
		Proceso: int32(g.proceso),
	}

	var wg sync.WaitGroup
	for _, par := range pares {
		wg.Add(1)
		go func(p *infopar) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_, err := p.Cliente.ActualizarVetos(ctx, &pb.ActualizacionVetos{
				Origen: origen,
				Vetos:  pbVetos,
			})
			if err != nil {
				g.logger.Errorf("Error replicando vetos a M%dP%d: %v", p.Maquina, p.Proceso, err)
			}
		}(par)
	}
	wg.Wait()
}

// RecuperarEstado implementa el protocolo de recuperación post-falla:
//  1. Solicita inventario y vetos a todos los pares en paralelo (ventana de 3 segundos).
//  2. Acepta hasta 3N-1 mensajes (N = total de procesos).
//  3. Verifica quorum: más de 2/3 de respuestas deben coincidir.
//  4. Si se cumple, aplica el estado con mayor consenso.
//  5. Si no, retorna error: el proceso no puede unirse al sistema.
func (g *Gestor) RecuperarEstado(proc *estado.Proceso) error {
	g.mu.RLock()
	pares := g.copiarPares()
	g.mu.RUnlock()

	if len(pares) == 0 {
		return fmt.Errorf("no hay pares disponibles para recuperar estado")
	}

	// N = pares + este proceso
	N := len(pares) + 1
	maxMensajes := 3*N - 1

	solicitante := &pb.IDProceso{
		Maquina: int32(g.maquina),
		Proceso: int32(g.proceso),
	}

	var (
		respuestas []respuestaPar
		mu         sync.Mutex
		wg         sync.WaitGroup
	)

	// Contexto con timeout de 3 segundos (especificación)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	for _, par := range pares {
		wg.Add(1)
		go func(p *infopar) {
			defer wg.Done()
			resp, err := p.Cliente.SolicitarEstado(ctx, &pb.SolicitudEstado{Solicitante: solicitante})
			if err != nil {
				g.logger.Errorf("Error solicitando estado a M%dP%d: %v", p.Maquina, p.Proceso, err)
				return
			}
			items := pbItemsAEstado(resp.Items)
			vetos := pbVetosAMapa(resp.Vetos)
			mu.Lock()
			respuestas = append(respuestas, respuestaPar{items: items, vetos: vetos})
			mu.Unlock()
		}(par)
	}
	wg.Wait()

	// Respetar límite de maxMensajes
	if len(respuestas) > maxMensajes {
		respuestas = respuestas[:maxMensajes]
	}

	if len(respuestas) == 0 {
		return fmt.Errorf("no se recibió ninguna respuesta de los pares")
	}

	g.logger.Infof("Recuperación: recibidas %d/%d respuestas (max=%d)",
		len(respuestas), N-1, maxMensajes)

	// Encontrar el grupo con más coincidencias
	mejorInventario, mejorVetos, maxCoincidencias := encontrarQuorum(respuestas)

	// Verificar que más de 2/3 de las respuestas coinciden
	umbral := float64(len(respuestas)) * 2.0 / 3.0
	g.logger.Infof("Quorum: %d coincidencias vs umbral %.2f (2/3 de %d respuestas)",
		maxCoincidencias, umbral, len(respuestas))

	if float64(maxCoincidencias) <= umbral {
		return fmt.Errorf("quorum no alcanzado: %d/%d coincidencias, necesita > %.2f",
			maxCoincidencias, len(respuestas), umbral)
	}

	// Aplicar el estado con quorum
	proc.ActualizarInventarioLocal(mejorInventario)
	proc.SetVetosDesdeReplica(mejorVetos)
	g.logger.Infof("Estado recuperado exitosamente con %d coincidencias", maxCoincidencias)
	return nil
}

// --- Helpers internos ---

// conectarAPar establece una conexión gRPC con un par y lo registra.
func (g *Gestor) conectarAPar(maquina, proceso int, direccion string) {
	clave := fmt.Sprintf("M%dP%d", maquina, proceso)
	g.mu.Lock()
	defer g.mu.Unlock()

	if _, existe := g.pares[clave]; existe {
		return
	}

	conn, err := grpc.Dial(direccion,
		grpc.WithInsecure(),
		grpc.WithBlock(),
		grpc.WithTimeout(2*time.Second),
	)
	if err != nil {
		g.logger.Errorf("No se pudo conectar a %s (%s): %v", clave, direccion, err)
		return
	}

	cliente := pb.NewExpendedoraClient(conn)
	g.pares[clave] = &infopar{
		Maquina:   maquina,
		Proceso:   proceso,
		Direccion: direccion,
		Cliente:   cliente,
		conn:      conn,
	}
	g.logger.Infof("Conectado a par %s en %s", clave, direccion)
}

// anunciarA envía un mensaje Registrar a todos los pares conocidos.
func (g *Gestor) anunciarA(miDireccion string) {
	g.mu.RLock()
	pares := g.copiarPares()
	g.mu.RUnlock()

	req := &pb.SolicitudRegistro{
		Id: &pb.IDProceso{
			Maquina: int32(g.maquina),
			Proceso: int32(g.proceso),
		},
		Direccion: miDireccion,
	}

	for _, par := range pares {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_, err := par.Cliente.Registrar(ctx, req)
		cancel()
		if err != nil {
			g.logger.Errorf("No se pudo anunciar a M%dP%d: %v", par.Maquina, par.Proceso, err)
		}
	}
}

// copiarPares retorna una copia de la lista de pares para iterar sin mantener el lock.
func (g *Gestor) copiarPares() []*infopar {
	lista := make([]*infopar, 0, len(g.pares))
	for _, p := range g.pares {
		lista = append(lista, p)
	}
	return lista
}

// encontrarQuorum busca el grupo de respuestas con mayor número de coincidencias exactas.
// Retorna el inventario y vetos del grupo ganador, y cuántas coincidencias tuvo.
func encontrarQuorum(respuestas []respuestaPar) ([]estado.Item, map[string]int, int) {
	type grupo struct {
		items []estado.Item
		vetos map[string]int
		count int
	}

	var grupos []*grupo
	for _, r := range respuestas {
		clave := serializarInventario(r.items)
		encontrado := false
		for _, g := range grupos {
			if serializarInventario(g.items) == clave {
				g.count++
				encontrado = true
				break
			}
		}
		if !encontrado {
			grupos = append(grupos, &grupo{
				items: r.items,
				vetos: r.vetos,
				count: 1,
			})
		}
	}

	sort.Slice(grupos, func(i, j int) bool {
		return grupos[i].count > grupos[j].count
	})

	if len(grupos) == 0 {
		return nil, nil, 0
	}
	return grupos[0].items, grupos[0].vetos, grupos[0].count
}

// serializarInventario convierte un inventario a string ordenado para comparaciones.
func serializarInventario(items []estado.Item) string {
	sorted := make([]estado.Item, len(items))
	copy(sorted, items)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Nombre < sorted[j].Nombre
	})
	data, _ := json.Marshal(sorted)
	return string(data)
}

// calcularPuerto es consistente con la fórmula en main.go:
// puerto = 50000 + (maquina-1)*100 + proceso
func calcularPuerto(maquina, proceso int) int {
	return 50000 + (maquina-1)*100 + proceso
}

// EntradaTopologia representa un proceso en el archivo topologia.json.
type EntradaTopologia struct {
	Maquina   int    `json:"maquina"`
	Proceso   int    `json:"proceso"`
	Direccion string `json:"direccion"`
}

// cargarTopologia lee el archivo topologia.json generado por el script bash.
func cargarTopologia() ([]EntradaTopologia, error) {
	data, err := os.ReadFile(archivoTopologia)
	if err != nil {
		return nil, err
	}
	var top []EntradaTopologia
	if err := json.Unmarshal(data, &top); err != nil {
		return nil, err
	}
	return top, nil
}

// --- Conversiones entre tipos protobuf y tipos internos ---

func pbItemsAEstado(pbItems []*pb.ItemInventario) []estado.Item {
	items := make([]estado.Item, len(pbItems))
	for i, it := range pbItems {
		items[i] = estado.Item{Nombre: it.Nombre, Cantidad: int(it.Cantidad)}
	}
	return items
}

func estadoItemsAPb(items []estado.Item) []*pb.ItemInventario {
	pbItems := make([]*pb.ItemInventario, len(items))
	for i, it := range items {
		pbItems[i] = &pb.ItemInventario{Nombre: it.Nombre, Cantidad: int32(it.Cantidad)}
	}
	return pbItems
}

func pbVetosAMapa(pbVetos []*pb.Veto) map[string]int {
	m := make(map[string]int, len(pbVetos))
	for _, v := range pbVetos {
		m[v.Persona] = int(v.Counter)
	}
	return m
}

func mapaVetosAPb(vetos map[string]int) []*pb.Veto {
	pbVetos := make([]*pb.Veto, 0, len(vetos))
	for p, c := range vetos {
		pbVetos = append(pbVetos, &pb.Veto{Persona: p, Counter: int32(c)})
	}
	return pbVetos
}
