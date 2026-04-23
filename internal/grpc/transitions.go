package grpc

import (
	"time"

	"github.com/ridakaddir/apitwin/internal/config"
	"github.com/ridakaddir/apitwin/internal/transitions"
)

// grpcTransitionState is the gRPC-flavoured instantiation of the shared
// generic state machine. The key is just route.Match since gRPC routes do
// not have an HTTP method prefix.
type grpcTransitionState = transitions.State[*config.GRPCRoute]

func grpcRouteKey(r *config.GRPCRoute) string { return r.Match }

// newGRPCTransitionState constructs a state machine seeded with the gRPC
// key function (match path only).
func newGRPCTransitionState() *grpcTransitionState {
	return transitions.NewState[*config.GRPCRoute](grpcRouteKey)
}

// newGRPCPersistentTransitionState constructs a persistent state machine
// that write-through saves FirstHit mutations under the "grpc" scope.
func newGRPCPersistentTransitionState(p transitions.Persister, seed map[string]time.Time) *grpcTransitionState {
	return transitions.NewPersistentState[*config.GRPCRoute](grpcRouteKey, "grpc", p, seed)
}
