// Package grpcserver implementa el servidor gRPC de la expendedora.
// Cada proceso ejecuta un servidor gRPC que responde a solicitudes
// de los otros procesos del sistema (replicación, sincronización, recuperación).
package grpcserver

import (
	"context"
	"fmt"
	"net"

	"google.golang.org/grpc"

	"tarea3/internal/estado"
	"tarea3/internal/logutil"
	pb "tarea3/proto/expendedorapb"
)

// Servidor implementa el servicio gRPC ExpendedoraServer.
type Servidor struct {
	pb.UnimplementedExpendedoraServer
	proc   *estado.Proceso
	logger *logutil.Logger

	// canal para notificar registro de nuevos pares
	CanalRegistro chan *pb.SolicitudRegistro
}

// GRPCServer encapsula el servidor gRPC y permite detenerlo.
type GRPCServer struct {
	srv *grpc.Server
}

// Iniciar crea y arranca el servidor gRPC en el puerto indicado.
// Retorna un GRPCServer y el Servidor de aplicación para que pares pueda usar el canal.
func Iniciar(proc *estado.Proceso, puerto int, logger *logutil.Logger) (*GRPCServer, error) {
	escucha, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", puerto))
	if err != nil {
		return nil, fmt.Errorf("no se puede escuchar en puerto %d: %w", puerto, err)
	}

	s := &Servidor{
		proc:          proc,
		logger:        logger,
		CanalRegistro: make(chan *pb.SolicitudRegistro, 100),
	}

	grpcSrv := grpc.NewServer()
	pb.RegisterExpendedoraServer(grpcSrv, s)

	// Guardar referencia al servidor de aplicación para que pares lo pueda usar
	servidorApp = s

	go func() {
		if err := grpcSrv.Serve(escucha); err != nil {
			logger.Errorf("Servidor gRPC detuvo: %v", err)
		}
	}()

	return &GRPCServer{srv: grpcSrv}, nil
}

// servidorApp es la instancia global del servidor de aplicación.
// Permite al paquete pares acceder al canal de registro.
var servidorApp *Servidor

// ObtenerServidorApp retorna la instancia del servidor de aplicación.
// Usada por el paquete pares para suscribirse al canal de registro.
func ObtenerServidorApp() *Servidor {
	return servidorApp
}

// Detener para el servidor gRPC de forma elegante.
func (g *GRPCServer) Detener() {
	g.srv.GracefulStop()
}

// --- Implementación de métodos gRPC ---

// Registrar es llamado por un proceso remoto para anunciar su existencia.
// El mensaje se reenvía al canal para que el gestor de pares lo procese.
func (s *Servidor) Registrar(_ context.Context, req *pb.SolicitudRegistro) (*pb.RespuestaRegistro, error) {
	s.logger.Infof("Registro recibido de M%dP%d en %s",
		req.Id.Maquina, req.Id.Proceso, req.Direccion)
	s.CanalRegistro <- req
	return &pb.RespuestaRegistro{Ok: true}, nil
}

// ActualizarInventario recibe la réplica de inventario de un proceso remoto
// y la almacena localmente. Si el inventario está infectado se rechaza con log.
func (s *Servidor) ActualizarInventario(_ context.Context, req *pb.ActualizacionInventario) (*pb.RespuestaRegistro, error) {
	items := pbItemsAEstado(req.Items)
	s.proc.SetReplicaInventario(int(req.Origen.Maquina), int(req.Origen.Proceso), items)
	s.logger.Infof("Inventario recibido de M%dP%d (%d productos)",
		req.Origen.Maquina, req.Origen.Proceso, len(items))
	return &pb.RespuestaRegistro{Ok: true}, nil
}

// ActualizarVetos recibe la lista de vetos de un proceso remoto y actualiza la local.
// Se usa sincronización por timestamp implícito (last-write-wins simplificado).
func (s *Servidor) ActualizarVetos(_ context.Context, req *pb.ActualizacionVetos) (*pb.RespuestaRegistro, error) {
	vetos := pbVetosAMapa(req.Vetos)
	s.proc.SetVetosDesdeReplica(vetos)
	s.logger.ActualizarLogVetos(vetos)
	s.logger.Infof("Vetos actualizados desde M%dP%d (%d vetos)",
		req.Origen.Maquina, req.Origen.Proceso, len(vetos))
	return &pb.RespuestaRegistro{Ok: true}, nil
}

// SolicitarEstado retorna el estado completo de este proceso al solicitante
// (usado durante la recuperación de un proceso caído).
// Si este proceso está infectado, envía el inventario alterado.
func (s *Servidor) SolicitarEstado(_ context.Context, req *pb.SolicitudEstado) (*pb.RespuestaEstado, error) {
	s.logger.Infof("Solicitud de estado de M%dP%d",
		req.Solicitante.Maquina, req.Solicitante.Proceso)

	items := estadoItemsAPb(s.proc.InventarioParaEnvio())
	vetos := mapaVetosAPb(s.proc.ObtenerVetos())

	return &pb.RespuestaEstado{
		Origen: &pb.IDProceso{
			Maquina: int32(s.proc.Maquina),
			Proceso: int32(s.proc.ID),
		},
		Items: items,
		Vetos: vetos,
	}, nil
}

// Sincronizar responde a un ping de sincronización periódica.
func (s *Servidor) Sincronizar(_ context.Context, req *pb.PingSinc) (*pb.PongSinc, error) {
	return &pb.PongSinc{Ok: true}, nil
}

// --- Conversiones entre tipos pb y tipos internos ---

// pbItemsAEstado convierte items protobuf a items de estado interno.
func pbItemsAEstado(pbItems []*pb.ItemInventario) []estado.Item {
	items := make([]estado.Item, len(pbItems))
	for i, it := range pbItems {
		items[i] = estado.Item{Nombre: it.Nombre, Cantidad: int(it.Cantidad)}
	}
	return items
}

// estadoItemsAPb convierte items de estado interno a items protobuf.
func estadoItemsAPb(items []estado.Item) []*pb.ItemInventario {
	pbItems := make([]*pb.ItemInventario, len(items))
	for i, it := range items {
		pbItems[i] = &pb.ItemInventario{Nombre: it.Nombre, Cantidad: int32(it.Cantidad)}
	}
	return pbItems
}

// pbVetosAMapa convierte vetos protobuf a mapa persona->counter.
func pbVetosAMapa(pbVetos []*pb.Veto) map[string]int {
	m := make(map[string]int, len(pbVetos))
	for _, v := range pbVetos {
		m[v.Persona] = int(v.Counter)
	}
	return m
}

// mapaVetosAPb convierte mapa persona->counter a vetos protobuf.
func mapaVetosAPb(vetos map[string]int) []*pb.Veto {
	pbVetos := make([]*pb.Veto, 0, len(vetos))
	for p, c := range vetos {
		pbVetos = append(pbVetos, &pb.Veto{Persona: p, Counter: int32(c)})
	}
	return pbVetos
}
