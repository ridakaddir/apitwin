package proxy

import (
	"github.com/ridakaddir/apitwin/internal/config"
	"github.com/ridakaddir/apitwin/internal/transitions"
)

// transitionState is a REST-flavoured instantiation of the shared generic
// state machine. The key format is "<METHOD> <match>" (e.g. "GET /orders/*")
// so the same pattern can have different sequences per HTTP verb.
type transitionState = transitions.State[*config.Route]

// newTransitionState constructs a state machine seeded with the REST key
// function that includes the HTTP method.
func newTransitionState() *transitionState {
	return transitions.NewState(routeKey)
}

// routeKey returns the unique key for a route. Exported for tests.
func routeKey(route *config.Route) string {
	return route.Method + " " + route.Match
}
