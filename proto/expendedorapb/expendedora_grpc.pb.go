// Code generated manually - gRPC service definitions for expendedora

package expendedorapb

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ExpendedoraClient is the client API for Expendedora service.
type ExpendedoraClient interface {
	Registrar(ctx context.Context, in *SolicitudRegistro, opts ...grpc.CallOption) (*RespuestaRegistro, error)
	ActualizarInventario(ctx context.Context, in *ActualizacionInventario, opts ...grpc.CallOption) (*RespuestaRegistro, error)
	ActualizarVetos(ctx context.Context, in *ActualizacionVetos, opts ...grpc.CallOption) (*RespuestaRegistro, error)
	SolicitarEstado(ctx context.Context, in *SolicitudEstado, opts ...grpc.CallOption) (*RespuestaEstado, error)
	Sincronizar(ctx context.Context, in *PingSinc, opts ...grpc.CallOption) (*PongSinc, error)
}

type expendedoraClient struct {
	cc grpc.ClientConnInterface
}

func NewExpendedoraClient(cc grpc.ClientConnInterface) ExpendedoraClient {
	return &expendedoraClient{cc}
}

func (c *expendedoraClient) Registrar(ctx context.Context, in *SolicitudRegistro, opts ...grpc.CallOption) (*RespuestaRegistro, error) {
	out := new(RespuestaRegistro)
	err := c.cc.Invoke(ctx, "/expendedora.Expendedora/Registrar", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *expendedoraClient) ActualizarInventario(ctx context.Context, in *ActualizacionInventario, opts ...grpc.CallOption) (*RespuestaRegistro, error) {
	out := new(RespuestaRegistro)
	err := c.cc.Invoke(ctx, "/expendedora.Expendedora/ActualizarInventario", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *expendedoraClient) ActualizarVetos(ctx context.Context, in *ActualizacionVetos, opts ...grpc.CallOption) (*RespuestaRegistro, error) {
	out := new(RespuestaRegistro)
	err := c.cc.Invoke(ctx, "/expendedora.Expendedora/ActualizarVetos", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *expendedoraClient) SolicitarEstado(ctx context.Context, in *SolicitudEstado, opts ...grpc.CallOption) (*RespuestaEstado, error) {
	out := new(RespuestaEstado)
	err := c.cc.Invoke(ctx, "/expendedora.Expendedora/SolicitarEstado", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *expendedoraClient) Sincronizar(ctx context.Context, in *PingSinc, opts ...grpc.CallOption) (*PongSinc, error) {
	out := new(PongSinc)
	err := c.cc.Invoke(ctx, "/expendedora.Expendedora/Sincronizar", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ExpendedoraServer is the server API for Expendedora service.
type ExpendedoraServer interface {
	Registrar(context.Context, *SolicitudRegistro) (*RespuestaRegistro, error)
	ActualizarInventario(context.Context, *ActualizacionInventario) (*RespuestaRegistro, error)
	ActualizarVetos(context.Context, *ActualizacionVetos) (*RespuestaRegistro, error)
	SolicitarEstado(context.Context, *SolicitudEstado) (*RespuestaEstado, error)
	Sincronizar(context.Context, *PingSinc) (*PongSinc, error)
}

// UnimplementedExpendedoraServer can be embedded to have forward compatible implementations.
type UnimplementedExpendedoraServer struct{}

func (UnimplementedExpendedoraServer) Registrar(context.Context, *SolicitudRegistro) (*RespuestaRegistro, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Registrar not implemented")
}
func (UnimplementedExpendedoraServer) ActualizarInventario(context.Context, *ActualizacionInventario) (*RespuestaRegistro, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ActualizarInventario not implemented")
}
func (UnimplementedExpendedoraServer) ActualizarVetos(context.Context, *ActualizacionVetos) (*RespuestaRegistro, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ActualizarVetos not implemented")
}
func (UnimplementedExpendedoraServer) SolicitarEstado(context.Context, *SolicitudEstado) (*RespuestaEstado, error) {
	return nil, status.Errorf(codes.Unimplemented, "method SolicitarEstado not implemented")
}
func (UnimplementedExpendedoraServer) Sincronizar(context.Context, *PingSinc) (*PongSinc, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Sincronizar not implemented")
}

func RegisterExpendedoraServer(s *grpc.Server, srv ExpendedoraServer) {
	s.RegisterService(&_Expendedora_serviceDesc, srv)
}

func _Expendedora_Registrar_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(SolicitudRegistro)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ExpendedoraServer).Registrar(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/expendedora.Expendedora/Registrar"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ExpendedoraServer).Registrar(ctx, req.(*SolicitudRegistro))
	}
	return interceptor(ctx, in, info, handler)
}

func _Expendedora_ActualizarInventario_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ActualizacionInventario)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ExpendedoraServer).ActualizarInventario(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/expendedora.Expendedora/ActualizarInventario"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ExpendedoraServer).ActualizarInventario(ctx, req.(*ActualizacionInventario))
	}
	return interceptor(ctx, in, info, handler)
}

func _Expendedora_ActualizarVetos_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ActualizacionVetos)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ExpendedoraServer).ActualizarVetos(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/expendedora.Expendedora/ActualizarVetos"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ExpendedoraServer).ActualizarVetos(ctx, req.(*ActualizacionVetos))
	}
	return interceptor(ctx, in, info, handler)
}

func _Expendedora_SolicitarEstado_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(SolicitudEstado)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ExpendedoraServer).SolicitarEstado(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/expendedora.Expendedora/SolicitarEstado"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ExpendedoraServer).SolicitarEstado(ctx, req.(*SolicitudEstado))
	}
	return interceptor(ctx, in, info, handler)
}

func _Expendedora_Sincronizar_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(PingSinc)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ExpendedoraServer).Sincronizar(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/expendedora.Expendedora/Sincronizar"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ExpendedoraServer).Sincronizar(ctx, req.(*PingSinc))
	}
	return interceptor(ctx, in, info, handler)
}

var _Expendedora_serviceDesc = grpc.ServiceDesc{
	ServiceName: "expendedora.Expendedora",
	HandlerType: (*ExpendedoraServer)(nil),
	Methods: []grpc.MethodDesc{
		{MethodName: "Registrar", Handler: _Expendedora_Registrar_Handler},
		{MethodName: "ActualizarInventario", Handler: _Expendedora_ActualizarInventario_Handler},
		{MethodName: "ActualizarVetos", Handler: _Expendedora_ActualizarVetos_Handler},
		{MethodName: "SolicitarEstado", Handler: _Expendedora_SolicitarEstado_Handler},
		{MethodName: "Sincronizar", Handler: _Expendedora_Sincronizar_Handler},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "expendedora.proto",
}
