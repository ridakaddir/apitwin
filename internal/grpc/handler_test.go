package grpc

import (
	"testing"
	"time"

	"github.com/ridakaddir/apitwin/internal/config"
)

func TestResolveCase_TransitionCaseNotInRoute_UsesFallback(t *testing.T) {
	// Simulate a create route where transitions define background-only cases
	// (e.g. "provisioning", "ready") that don't exist as request-time cases.
	// The fallback "created" case should be returned instead.
	route := &config.GRPCRoute{
		Match:    "/continent.v1.CountryService/CreateCity",
		Fallback: "created",
		Transitions: []config.Transition{
			{Case: "provisioning", Duration: 5},
			{Case: "ready"},
		},
		Cases: map[string]config.Case{
			"created": {Status: 0, Persist: true, Merge: "append"},
		},
	}

	h := &handler{
		transitions: newGRPCTransitionState(),
	}

	// Seed the transition state so it returns "provisioning".
	h.transitions.FirstHit[route.Match] = time.Now()

	got := h.resolveCase(route, nil)
	if got != "created" {
		t.Errorf("resolveCase = %q, want %q (fallback); transition case should be skipped when not in route.Cases", got, "created")
	}
}

func TestResolveCase_TransitionCaseInRoute_UsesTransition(t *testing.T) {
	// When the transition case exists in route.Cases, it should be used.
	route := &config.GRPCRoute{
		Match:    "/continent.v1.CountryService/GetCity",
		Fallback: "default",
		Transitions: []config.Transition{
			{Case: "loading", Duration: 5},
			{Case: "ready"},
		},
		Cases: map[string]config.Case{
			"default": {Status: 0},
			"loading": {Status: 0},
			"ready":   {Status: 0},
		},
	}

	h := &handler{
		transitions: newGRPCTransitionState(),
	}

	// Seed the transition state so it returns "loading".
	h.transitions.FirstHit[route.Match] = time.Now()

	got := h.resolveCase(route, nil)
	if got != "loading" {
		t.Errorf("resolveCase = %q, want %q", got, "loading")
	}
}
