package grpc

import (
	"github.com/ridakaddir/apitwin/internal/config"
	"github.com/ridakaddir/apitwin/internal/transitions"
)

// grpcTransitionState is the gRPC-flavoured instantiation of the shared
// generic state machine. The key is just route.Match since gRPC routes do
// not have an HTTP method prefix.
type grpcTransitionState = transitions.State[*config.GRPCRoute]

// newGRPCTransitionState constructs a state machine seeded with the gRPC
// key function (match path only).
func newGRPCTransitionState() *grpcTransitionState {
	return transitions.NewState[*config.GRPCRoute](func(r *config.GRPCRoute) string {
		return r.Match
	})
}
