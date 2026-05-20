package grpc

import (
	"strings"

	"github.com/ridakaddir/apitwin/internal/validation"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// grpcInvalidArgument builds the canonical INVALID_ARGUMENT response for a
// failed payload validation. The status message is a single line joining
// every violation; per-field details ride in a BadRequest detail block so
// canonical gRPC clients can render each violation against the right
// field. Mirrors the HTTP 400 + violations JSON on the REST side.
func grpcInvalidArgument(violations []validation.Violation) error {
	if len(violations) == 0 {
		return status.Error(codes.InvalidArgument, "validation failed")
	}
	msgs := make([]string, 0, len(violations))
	br := &errdetails.BadRequest{}
	for _, v := range violations {
		msgs = append(msgs, v.Message)
		br.FieldViolations = append(br.FieldViolations, &errdetails.BadRequest_FieldViolation{
			Field:       v.Field,
			Description: v.Message,
		})
	}
	st := status.New(codes.InvalidArgument, strings.Join(msgs, "; "))
	if detailed, err := st.WithDetails(br); err == nil {
		return detailed.Err()
	}
	return st.Err()
}
