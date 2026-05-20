package proxy

import (
	"encoding/json"
	"net/http"

	"github.com/ridakaddir/apitwin/internal/logger"
	"github.com/ridakaddir/apitwin/internal/validation"
)

// writeValidationError emits the canonical 400 response for a failed
// payload validation: HTTP status 400, application/json, with the list of
// per-field violations. Shape mirrors the gRPC counterpart so devs see
// the same field/rule/message triplet on either transport.
func writeValidationError(w http.ResponseWriter, violations []validation.Violation) {
	logger.SetSource(w, logger.SourceStub)
	body := map[string]any{
		"error":      "validation failed",
		"violations": violations,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(body)
}

