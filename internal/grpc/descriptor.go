package grpc

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/jhump/protoreflect/desc"            //nolint:staticcheck
	"github.com/jhump/protoreflect/desc/protoparse" //nolint:staticcheck
	"github.com/jhump/protoreflect/dynamic"         //nolint:staticcheck
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
)

// Registry compiles .proto files at startup and provides method/message lookup
// and JSON ↔ proto transcoding without requiring protoc code generation.
type Registry struct {
	methods  map[string]*desc.MethodDescriptor // key: "/package.Service/Method"
	services map[string]*desc.ServiceDescriptor
	files    []*desc.FileDescriptor
	factory  *dynamic.MessageFactory
}

// NewRegistry parses the given .proto files and builds a method registry.
// importPaths are directories searched when resolving proto imports.
func NewRegistry(protoFiles []string, importPaths []string) (*Registry, error) {
	if len(protoFiles) == 0 {
		return &Registry{
			methods: make(map[string]*desc.MethodDescriptor),
			factory: dynamic.NewMessageFactoryWithDefaults(),
		}, nil
	}

	p := protoparse.Parser{
		ImportPaths:           importPaths,
		InferImportPaths:      true,
		IncludeSourceCodeInfo: false,
	}

	fds, err := p.ParseFiles(protoFiles...)
	if err != nil {
		return nil, fmt.Errorf("parsing proto files: %w", err)
	}

	methods := make(map[string]*desc.MethodDescriptor)
	services := make(map[string]*desc.ServiceDescriptor)
	for _, fd := range fds {
		for _, svc := range fd.GetServices() {
			services[svc.GetFullyQualifiedName()] = svc
			for _, m := range svc.GetMethods() {
				key := fmt.Sprintf("/%s/%s", svc.GetFullyQualifiedName(), m.GetName())
				methods[key] = m
			}
		}
	}

	return &Registry{
		methods:  methods,
		services: services,
		files:    fds,
		factory:  dynamic.NewMessageFactoryWithDefaults(),
	}, nil
}

// FindMethod returns the MethodDescriptor for the given full gRPC method path.
// Returns nil, nil when the method is not in any loaded .proto file.
func (r *Registry) FindMethod(fullMethod string) (*desc.MethodDescriptor, error) {
	md, ok := r.methods[fullMethod]
	if !ok {
		return nil, nil
	}
	return md, nil
}

// Methods returns all registered full method paths.
func (r *Registry) Methods() []string {
	out := make([]string, 0, len(r.methods))
	for k := range r.methods {
		out = append(out, k)
	}
	return out
}

// ServiceNames returns the fully-qualified service names from loaded protos.
// Used to populate the gRPC reflection service.
func (r *Registry) ServiceNames() []string {
	out := make([]string, 0, len(r.services))
	for name := range r.services {
		out = append(out, name)
	}
	return out
}

// DescriptorResolver returns a protodesc.Resolver backed by the loaded file
// descriptors. Used to wire proto schemas into the gRPC reflection service.
func (r *Registry) DescriptorResolver() protodesc.Resolver {
	files := &protoregistry.Files{}
	for _, fd := range r.files {
		// Walk every file and all its transitive dependencies.
		_ = registerFile(files, fd)
	}
	return files
}

// registerFile adds fd and all its imports to the registry, skipping duplicates.
func registerFile(reg *protoregistry.Files, fd *desc.FileDescriptor) error {
	pfd := fd.UnwrapFile()
	if _, err := reg.FindFileByPath(pfd.Path()); err == nil {
		return nil // already registered
	}
	// Register imports first.
	for _, dep := range fd.GetDependencies() {
		if err := registerFile(reg, dep); err != nil && !errors.Is(err, protoregistry.NotFound) {
			return err
		}
	}
	return reg.RegisterFile(pfd)
}

// DecodeRequest deserialises the raw protobuf bytes of a request message into
// a map[string]interface{} using protojson field names, ready for condition
// evaluation (the same evalCondition / extractValue logic used for REST).
func (r *Registry) DecodeRequest(md *desc.MethodDescriptor, b []byte) (map[string]interface{}, error) {
	msg := r.factory.NewDynamicMessage(md.GetInputType())
	if err := msg.Unmarshal(b); err != nil {
		return nil, fmt.Errorf("unmarshal request: %w", err)
	}

	jsonBytes, err := msg.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("marshal request to json: %w", err)
	}

	var out map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &out); err != nil {
		return nil, fmt.Errorf("parse request json: %w", err)
	}
	return out, nil
}

// FindRepeatedField inspects the response message for a single repeated message
// field and returns its JSON (camelCase) name. Used to auto-wrap directory
// aggregation arrays into the correct response object field.
// Returns "" if there is no single repeated message field.
func (r *Registry) FindRepeatedField(md *desc.MethodDescriptor) string {
	outType := md.GetOutputType()
	var candidate string
	for _, f := range outType.GetFields() {
		if f.IsRepeated() && !f.IsMap() {
			if candidate != "" {
				return "" // ambiguous — more than one repeated field
			}
			candidate = f.GetJSONName()
		}
	}
	return candidate
}

// RequestEntity looks up the request field that holds the entity the RPC is
// operating on, and (when applicable) the response wrapper field it should
// be returned under. It covers two Google API conventions:
//
//  1. Direct-entity response — the response message IS the entity. Returns
//     (source, "", false) where source is the JSON name of the unique
//     request field whose message type equals the response output type:
//
//	     rpc UpdateDatabaseInstance(UpdateDatabaseInstanceRequest) returns (DatabaseInstance)
//	     UpdateDatabaseInstanceRequest { string name = 1; DatabaseInstance database_instance = 2; }
//	     → ("databaseInstance", "", false)
//
//  2. Response-wrapped — the response wraps the entity under a single
//     message field. Returns (source, wrap, false) where source is the
//     matching request field and wrap is the response wrapper field:
//
//	     rpc UpdateGateway(UpdateGatewayRequest) returns (UpdateGatewayResponse)
//	     UpdateGatewayResponse { Gateway gateway = 1; }
//	     UpdateGatewayRequest  { GatewayParent parent = 1; string gateway_name = 2; Gateway gateway = 3; }
//	     → ("gateway", "gateway", false)
//
// Shape 1 is tried first. Shape 2 is the fallback: it applies only when the
// response type has exactly one non-repeated, non-map message field — that
// field's type is then treated as the entity type. Scalars, repeated, and
// map fields in the request are ignored by both shapes, so a prefix-
// colliding scalar like `string gateway_name = 2;` cannot trip the match.
//
// Returns ("", "", false) when no shape matches or md is nil (silent skip:
// the caller should leave source/wrap at the explicit config value, which
// defaults to untouched merge behaviour). Returns ("", "", true) when the
// matching shape has more than one request field of the entity type
// (ambiguous — caller should warn so the user can disambiguate by setting
// `source` explicitly). Matching is always done on fully-qualified message
// names.
func (r *Registry) RequestEntity(md *desc.MethodDescriptor) (source, wrap string, ambiguous bool) {
	if md == nil {
		return "", "", false
	}
	outType := md.GetOutputType()
	if outType == nil {
		return "", "", false
	}
	inType := md.GetInputType()
	if inType == nil {
		return "", "", false
	}

	// Shape 1: response IS the entity.
	if src, amb := uniqueRequestFieldOfType(inType, outType.GetFullyQualifiedName()); amb {
		return "", "", true
	} else if src != "" {
		return src, "", false
	}

	// Shape 2: response WRAPS a single entity field.
	wrapField := uniqueNonRepeatedMessageField(outType)
	if wrapField == nil {
		return "", "", false
	}
	entityFQN := wrapField.GetMessageType().GetFullyQualifiedName()
	src, amb := uniqueRequestFieldOfType(inType, entityFQN)
	if amb {
		return "", "", true
	}
	if src == "" {
		return "", "", false
	}
	return src, wrapField.GetJSONName(), false
}

// RequestEntityField is a backwards-compatible shim around RequestEntity
// that returns only the source field and ambiguity flag. New callers should
// use RequestEntity directly to also pick up the inferred wrap.
func (r *Registry) RequestEntityField(md *desc.MethodDescriptor) (name string, ambiguous bool) {
	src, _, amb := r.RequestEntity(md)
	return src, amb
}

// ResponseWrap returns the JSON name of the response type's unique wrapper
// field — the single non-repeated, non-map message field on the output type.
// Returns "" when the response has zero, two, or more such fields, or when
// the lone field is a scalar (shape 1 direct-entity response: the response
// IS the entity, no wrap needed).
//
// Used by applyGRPCPersist when the user has explicitly set c.Source but left
// c.Wrap empty: the source-driven extraction plus the response wrapper
// inference are independent concerns, so having an explicit source should
// not disable wrap inference from the response type.
func (r *Registry) ResponseWrap(md *desc.MethodDescriptor) string {
	if md == nil {
		return ""
	}
	outType := md.GetOutputType()
	if outType == nil {
		return ""
	}
	wrapField := uniqueNonRepeatedMessageField(outType)
	if wrapField == nil {
		return ""
	}
	return wrapField.GetJSONName()
}

// uniqueRequestFieldOfType returns the JSON name of the single non-repeated,
// non-map message-typed field in inType whose message type matches fqn, or
// ("", true) when more than one field matches (ambiguous). Scalars and
// repeated / map fields are always skipped, so request fields like
// `string gateway_name = 2;` cannot trigger a false match on "gateway".
func uniqueRequestFieldOfType(inType *desc.MessageDescriptor, fqn string) (name string, ambiguous bool) {
	var candidate string
	for _, f := range inType.GetFields() {
		if f.IsRepeated() || f.IsMap() {
			continue
		}
		mt := f.GetMessageType()
		if mt == nil {
			continue
		}
		if mt.GetFullyQualifiedName() != fqn {
			continue
		}
		if candidate != "" {
			return "", true
		}
		candidate = f.GetJSONName()
	}
	return candidate, false
}

// uniqueNonRepeatedMessageField returns the lone non-repeated, non-map
// message-typed field of outType, or nil when outType has zero, two, or
// more such fields. Used to detect "response wrapper" message shapes like
// UpdateGatewayResponse { Gateway gateway = 1; } — messages with additional
// scalar or repeated fields are deliberately not unwrapped because there is
// no safe way to pick one.
func uniqueNonRepeatedMessageField(outType *desc.MessageDescriptor) *desc.FieldDescriptor {
	var candidate *desc.FieldDescriptor
	for _, f := range outType.GetFields() {
		if f.IsRepeated() || f.IsMap() {
			return nil
		}
		if f.GetMessageType() == nil {
			return nil
		}
		if candidate != nil {
			return nil
		}
		candidate = f
	}
	return candidate
}

// EntityFieldNames returns the set of JSON and proto field names defined on the
// entity message identified by the wrap field of the response type. This is used
// to filter persist data so that only fields belonging to the entity message are
// written to disk (preventing routing fields from leaking into stubs).
// Returns nil if wrap is empty, not found, or does not point to a message type.
func (r *Registry) EntityFieldNames(md *desc.MethodDescriptor, wrap string) map[string]bool {
	if wrap == "" || md == nil {
		return nil
	}
	outType := md.GetOutputType()
	var entityDesc *desc.MessageDescriptor
	for _, f := range outType.GetFields() {
		if f.GetJSONName() == wrap || f.GetName() == wrap {
			entityDesc = f.GetMessageType()
			break
		}
	}
	if entityDesc == nil {
		return nil
	}
	fields := entityDesc.GetFields()
	allowed := make(map[string]bool, len(fields)*2)
	for _, f := range fields {
		allowed[f.GetJSONName()] = true
		allowed[f.GetName()] = true
	}
	return allowed
}

// EncodeResponse takes a JSON blob (protojson-compatible) and serialises it
// into wire bytes for the given method's output type.
func (r *Registry) EncodeResponse(md *desc.MethodDescriptor, jsonBytes []byte) ([]byte, error) {
	msg := r.factory.NewDynamicMessage(md.GetOutputType())
	if err := msg.UnmarshalJSON(jsonBytes); err != nil {
		return nil, fmt.Errorf("unmarshal response json: %w", err)
	}
	b, err := msg.Marshal()
	if err != nil {
		return nil, fmt.Errorf("marshal response proto: %w", err)
	}
	return b, nil
}

// EncodeRequest takes a JSON blob and serialises it into wire bytes for the
// given method's input type. Symmetric with DecodeRequest — used by the
// devtool invoker to send browser-originated requests to the gRPC server.
func (r *Registry) EncodeRequest(md *desc.MethodDescriptor, jsonBytes []byte) ([]byte, error) {
	msg := r.factory.NewDynamicMessage(md.GetInputType())
	if err := msg.UnmarshalJSON(jsonBytes); err != nil {
		return nil, fmt.Errorf("unmarshal request json: %w", err)
	}
	b, err := msg.Marshal()
	if err != nil {
		return nil, fmt.Errorf("marshal request proto: %w", err)
	}
	return b, nil
}

// DecodeResponse takes wire bytes for the given method's output type and
// returns them re-encoded as protojson. Symmetric with EncodeResponse.
func (r *Registry) DecodeResponse(md *desc.MethodDescriptor, b []byte) ([]byte, error) {
	msg := r.factory.NewDynamicMessage(md.GetOutputType())
	if err := msg.Unmarshal(b); err != nil {
		return nil, fmt.Errorf("unmarshal response proto: %w", err)
	}
	j, err := msg.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("marshal response json: %w", err)
	}
	return j, nil
}

// MethodSchema describes one RPC method for the devtool UI: method path,
// input/output type FQNs, streaming flag, and the list of top-level input
// fields so the UI can render a form or show a reference panel.
type MethodSchema struct {
	Method     string      `json:"method"`
	Service    string      `json:"service"`
	InputType  string      `json:"inputType"`
	OutputType string      `json:"outputType"`
	Streaming  bool        `json:"streaming"`
	Fields     []FieldInfo `json:"fields"`
}

// FieldInfo is a flattened view of a single message field suitable for JSON
// rendering in the devtool UI.
type FieldInfo struct {
	Name        string `json:"name"`
	JSONName    string `json:"jsonName"`
	Kind        string `json:"kind"`
	Repeated    bool   `json:"repeated"`
	MessageType string `json:"messageType,omitempty"`
	EnumType    string `json:"enumType,omitempty"`
}

// Schema returns a devtool-friendly summary of every method in the registry,
// ordered by method path for stable UI display.
func (r *Registry) Schema() []MethodSchema {
	out := make([]MethodSchema, 0, len(r.methods))
	for key, md := range r.methods {
		out = append(out, MethodSchema{
			Method:     key,
			Service:    md.GetService().GetFullyQualifiedName(),
			InputType:  md.GetInputType().GetFullyQualifiedName(),
			OutputType: md.GetOutputType().GetFullyQualifiedName(),
			Streaming:  md.IsClientStreaming() || md.IsServerStreaming(),
			Fields:     describeFields(md.GetInputType()),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Method < out[j].Method })
	return out
}

func describeFields(mt *desc.MessageDescriptor) []FieldInfo {
	if mt == nil {
		return nil
	}
	fields := mt.GetFields()
	out := make([]FieldInfo, 0, len(fields))
	for _, f := range fields {
		fi := FieldInfo{
			Name:     f.GetName(),
			JSONName: f.GetJSONName(),
			Kind:     fieldKind(f),
			Repeated: f.IsRepeated() && !f.IsMap(),
		}
		if sub := f.GetMessageType(); sub != nil {
			fi.MessageType = sub.GetFullyQualifiedName()
		}
		if en := f.GetEnumType(); en != nil {
			fi.EnumType = en.GetFullyQualifiedName()
		}
		out = append(out, fi)
	}
	return out
}

func fieldKind(f *desc.FieldDescriptor) string {
	if f.IsMap() {
		return "map"
	}
	switch f.GetType() {
	case descriptorpb.FieldDescriptorProto_TYPE_DOUBLE:
		return "double"
	case descriptorpb.FieldDescriptorProto_TYPE_FLOAT:
		return "float"
	case descriptorpb.FieldDescriptorProto_TYPE_INT64, descriptorpb.FieldDescriptorProto_TYPE_SFIXED64, descriptorpb.FieldDescriptorProto_TYPE_SINT64:
		return "int64"
	case descriptorpb.FieldDescriptorProto_TYPE_UINT64, descriptorpb.FieldDescriptorProto_TYPE_FIXED64:
		return "uint64"
	case descriptorpb.FieldDescriptorProto_TYPE_INT32, descriptorpb.FieldDescriptorProto_TYPE_SFIXED32, descriptorpb.FieldDescriptorProto_TYPE_SINT32:
		return "int32"
	case descriptorpb.FieldDescriptorProto_TYPE_UINT32, descriptorpb.FieldDescriptorProto_TYPE_FIXED32:
		return "uint32"
	case descriptorpb.FieldDescriptorProto_TYPE_BOOL:
		return "bool"
	case descriptorpb.FieldDescriptorProto_TYPE_STRING:
		return "string"
	case descriptorpb.FieldDescriptorProto_TYPE_BYTES:
		return "bytes"
	case descriptorpb.FieldDescriptorProto_TYPE_ENUM:
		return "enum"
	case descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, descriptorpb.FieldDescriptorProto_TYPE_GROUP:
		return "message"
	}
	return "unknown"
}
