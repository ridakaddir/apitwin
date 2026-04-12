package grpc

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Invoker is a local gRPC client used by the devtool UI so browsers — which
// cannot speak native gRPC — can exercise mocked methods through a
// JSON-in/JSON-out HTTP endpoint. It dials the apitwin gRPC server on
// localhost using the same raw-bytes codec the server installs, and
// transcodes request/response bodies through the shared *Registry.
type Invoker struct {
	registry *Registry
	target   string

	mu   sync.Mutex
	conn *grpc.ClientConn
}

func newInvoker(registry *Registry, target string) *Invoker {
	return &Invoker{registry: registry, target: target}
}

// Close releases the underlying client connection, if any.
func (i *Invoker) Close() error {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.conn == nil {
		return nil
	}
	err := i.conn.Close()
	i.conn = nil
	return err
}

// Schema returns the input/output descriptors for every loaded method.
// The return is typed as `any` to satisfy the proxy.GRPCInvoker interface
// without requiring that package to import internal/grpc.
func (i *Invoker) Schema() any {
	if i == nil || i.registry == nil {
		return []MethodSchema{}
	}
	return i.registry.Schema()
}

// InvokeResult carries the wire-level outcome of a JSON-shaped gRPC call.
// On a non-OK status, Response is nil and Code/Message describe the error;
// transport failures are returned as the Go error from Invoke.
type InvokeResult struct {
	Code     codes.Code      `json:"-"`
	CodeName string          `json:"code"`
	Message  string          `json:"message,omitempty"`
	Response json.RawMessage `json:"response,omitempty"`
	LatencyMs int64           `json:"latencyMs"`
}

// Invoke runs the unary call named by fullMethod with the given request body
// as JSON. It returns a non-nil result describing any outcome the server
// produced (including non-OK gRPC statuses); only transport or transcoding
// failures are returned as Go errors. The return is typed as `any` to
// satisfy the proxy.GRPCInvoker interface (see Schema for the rationale).
func (i *Invoker) Invoke(ctx context.Context, fullMethod string, md map[string]string, reqJSON []byte) (any, error) {
	if i == nil || i.registry == nil {
		return nil, fmt.Errorf("no proto registry loaded — start apitwin with --grpc-proto")
	}

	method, err := i.registry.FindMethod(fullMethod)
	if err != nil {
		return nil, err
	}
	if method == nil {
		return nil, fmt.Errorf("method %q not in any loaded --grpc-proto file", fullMethod)
	}
	if method.IsClientStreaming() || method.IsServerStreaming() {
		return nil, fmt.Errorf("streaming RPCs are not supported by the devtool client")
	}

	if len(reqJSON) == 0 {
		reqJSON = []byte("{}")
	}
	reqBytes, err := i.registry.EncodeRequest(method, reqJSON)
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}

	conn, err := i.dial()
	if err != nil {
		return nil, err
	}

	if len(md) > 0 {
		pairs := make([]string, 0, len(md)*2)
		for k, v := range md {
			pairs = append(pairs, k, v)
		}
		ctx = metadata.AppendToOutgoingContext(ctx, pairs...)
	}

	start := time.Now()
	stream, err := conn.NewStream(ctx, &grpc.StreamDesc{}, fullMethod)
	if err != nil {
		return toResult(err, start), nil
	}
	if err := stream.SendMsg(&reqBytes); err != nil {
		return toResult(err, start), nil
	}
	if err := stream.CloseSend(); err != nil {
		return toResult(err, start), nil
	}

	var rawResp []byte
	if err := stream.RecvMsg(&rawResp); err != nil {
		return toResult(err, start), nil
	}
	latency := time.Since(start)

	respJSON, err := i.registry.DecodeResponse(method, rawResp)
	if err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &InvokeResult{
		Code:      codes.OK,
		CodeName:  codes.OK.String(),
		Response:  respJSON,
		LatencyMs: latency.Milliseconds(),
	}, nil
}

func (i *Invoker) dial() (*grpc.ClientConn, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.conn != nil {
		return i.conn, nil
	}
	conn, err := grpc.NewClient(i.target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.ForceCodec(encoding.GetCodec("proto"))),
	)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", i.target, err)
	}
	i.conn = conn
	return conn, nil
}

func toResult(err error, start time.Time) *InvokeResult {
	latency := time.Since(start)
	if st, ok := status.FromError(err); ok {
		return &InvokeResult{
			Code:      st.Code(),
			CodeName:  st.Code().String(),
			Message:   st.Message(),
			LatencyMs: latency.Milliseconds(),
		}
	}
	return &InvokeResult{
		Code:      codes.Unknown,
		CodeName:  codes.Unknown.String(),
		Message:   err.Error(),
		LatencyMs: latency.Milliseconds(),
	}
}
