package grpc

import (
	"testing"

	"github.com/ridakaddir/apitwin/internal/validation"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestGRPCInvalidArgument_StatusCodeAndMessage(t *testing.T) {
	violations := []validation.Violation{
		{Field: "name", Rule: "required", Message: "name is required"},
		{Field: "population", Rule: "gte", Message: "population must be >= 0"},
	}
	err := grpcInvalidArgument(violations)
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status, got %T", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", st.Code())
	}
	if got := st.Message(); got != "name is required; population must be >= 0" {
		t.Fatalf("unexpected status message: %q", got)
	}
}

func TestGRPCInvalidArgument_BadRequestDetails(t *testing.T) {
	violations := []validation.Violation{
		{Field: "continent", Rule: "in", Message: `continent must be one of [africa, europe, asia]`},
	}
	err := grpcInvalidArgument(violations)
	st, _ := status.FromError(err)
	details := st.Details()
	if len(details) != 1 {
		t.Fatalf("expected 1 detail block, got %d", len(details))
	}
	br, ok := details[0].(*errdetails.BadRequest)
	if !ok {
		t.Fatalf("expected BadRequest detail, got %T", details[0])
	}
	if len(br.FieldViolations) != 1 {
		t.Fatalf("expected 1 field violation, got %d", len(br.FieldViolations))
	}
	fv := br.FieldViolations[0]
	if fv.Field != "continent" {
		t.Fatalf("expected field=continent, got %q", fv.Field)
	}
	if fv.Description != violations[0].Message {
		t.Fatalf("description mismatch: %q vs %q", fv.Description, violations[0].Message)
	}
}

func TestGRPCInvalidArgument_EmptyViolationsStillReturnsStatus(t *testing.T) {
	// Defensive: a caller that hits the helper with no violations (e.g. a
	// regression that drops the rule list) should still get a coherent
	// InvalidArgument error, not a nil or panic.
	err := grpcInvalidArgument(nil)
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", st.Code())
	}
	if st.Message() == "" {
		t.Fatal("expected non-empty status message")
	}
}
