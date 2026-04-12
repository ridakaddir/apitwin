package grpc

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/ridakaddir/apitwin/internal/config"
	"github.com/ridakaddir/apitwin/internal/logger"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	v1reflectiongrpc "google.golang.org/grpc/reflection/grpc_reflection_v1"
	v1alphareflectiongrpc "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
)

// ServerOptions holds all configuration for the gRPC server.
type ServerOptions struct {
	// ProtoFiles is the list of .proto file paths to load at startup.
	ProtoFiles []string
	// ImportPaths are extra directories to search when resolving proto imports.
	// When empty, the directory of each ProtoFile is used automatically.
	ImportPaths []string
	// Target is the upstream gRPC address for proxy mode (e.g. "localhost:9090").
	// Empty means mock-only mode.
	Target string
	// Port is the TCP port the gRPC server listens on (default 50051).
	Port int
	// Loader provides the live config and config directory path.
	// StubRoot() returns the directory stub I/O should resolve against —
	// normally the runtime mirror dir, or ConfigDir() in legacy mode.
	Loader interface {
		Get() *config.Config
		ConfigDir() string
		StubRoot() string
	}
}

// Server wraps a grpc.Server and its supporting components.
type Server struct {
	opts    ServerOptions
	srv     *grpc.Server
	handler *handler

	invokerOnce sync.Once
	invoker     *Invoker
}

// NewServer initialises the gRPC server: parses proto files, wires the handler,
// and creates the underlying grpc.Server. Call Start to begin accepting connections.
func NewServer(opts ServerOptions) (*Server, error) {
	// Install the raw-bytes codec here, not in an init(), so HTTP-only runs
	// are unaffected by the global gRPC codec override.
	InstallCodec()

	registry, err := NewRegistry(opts.ProtoFiles, opts.ImportPaths)
	if err != nil {
		return nil, fmt.Errorf("building proto registry: %w", err)
	}

	h := newHandler(opts.Loader, registry, opts.Target)

	srv := grpc.NewServer(
		grpc.UnknownServiceHandler(h.serve),
	)

	// Register the reflection service with our proto-aware descriptor resolver
	// and service name list. This lets grpcurl / grpc-ui discover our services
	// even though we use UnknownServiceHandler instead of registered stubs.
	svcNames := registry.ServiceNames()
	if len(svcNames) > 0 {
		svrOpts := reflection.ServerOptions{
			Services:           &protoServiceInfoProvider{names: svcNames},
			DescriptorResolver: registry.DescriptorResolver(),
		}
		// Register both v1 and v1alpha so all versions of grpcurl work.
		v1alphareflectiongrpc.RegisterServerReflectionServer(srv, reflection.NewServer(svrOpts))
		v1reflectiongrpc.RegisterServerReflectionServer(srv, reflection.NewServerV1(svrOpts))
	} else {
		// No proto loaded — fall back to standard reflection.
		reflection.Register(srv)
	}

	return &Server{
		opts:    opts,
		srv:     srv,
		handler: h,
	}, nil
}

// Start listens on the configured port and serves gRPC requests until ctx is cancelled.
func (s *Server) Start(ctx context.Context) error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", s.opts.Port))
	if err != nil {
		return fmt.Errorf("gRPC listen on port %d: %w", s.opts.Port, err)
	}

	errCh := make(chan error, 1)
	go func() {
		methods := s.handler.registry.Methods()
		logger.Info("gRPC server running",
			"port", s.opts.Port,
			"proto_methods", len(methods),
			"target", s.opts.Target,
		)
		if err := s.srv.Serve(lis); err != nil {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		s.srv.GracefulStop()
		s.Stop()
		return nil
	case err := <-errCh:
		return err
	}
}

// Invoker returns a devtool-facing gRPC client for this server, constructed
// on first use. The client dials localhost:<Port> and shares the server's
// proto registry so it can transcode JSON request/response bodies on behalf
// of browser callers.
func (s *Server) Invoker() *Invoker {
	s.invokerOnce.Do(func() {
		s.invoker = newInvoker(s.handler.registry, fmt.Sprintf("localhost:%d", s.opts.Port))
	})
	return s.invoker
}

// NotifyReload resets transition state after a config hot-reload.
// Called from the config loader's onChange callback.
func (s *Server) NotifyReload() {
	s.handler.resetTransitions()
	s.handler.scheduler.Reset(context.Background())
}

// Stop cancels pending transition schedulers and waits for goroutines to finish.
// Called on server shutdown.
func (s *Server) Stop() {
	s.handler.scheduler.Stop()
	if s.invoker != nil {
		_ = s.invoker.Close()
	}
}

// protoServiceInfoProvider satisfies reflection.ServiceInfoProvider using our
// known service names. The grpc.ServiceInfo values are empty — reflection only
// needs the names to populate the ListServices response.
type protoServiceInfoProvider struct {
	names []string
}

func (p *protoServiceInfoProvider) GetServiceInfo() map[string]grpc.ServiceInfo {
	out := make(map[string]grpc.ServiceInfo, len(p.names))
	for _, name := range p.names {
		out[name] = grpc.ServiceInfo{}
	}
	return out
}
