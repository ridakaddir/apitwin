package grpc

import mocktmpl "github.com/ridakaddir/apitwin/internal/template"

// renderGRPCTemplate processes {{uuid}}, {{now}}, {{timestamp}} tokens in a
// JSON string. Thin wrapper over the shared internal/template package so
// gRPC and REST share a single token renderer.
func renderGRPCTemplate(s string) (string, error) {
	return mocktmpl.RenderTokens(s)
}
