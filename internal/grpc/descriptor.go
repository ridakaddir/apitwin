package grpc

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jhump/protoreflect/desc"            //nolint:staticcheck
	"github.com/jhump/protoreflect/desc/protoparse" //nolint:staticcheck
	"github.com/jhump/protoreflect/dynamic"         //nolint:staticcheck
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoregistry"
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

// RequestEntityField returns the JSON name of the unique request input
// field whose message type equals the response output type. This is the
// Google API convention for Update<X>Request, e.g.
//
//	UpdateDatabaseInstanceRequest { string name = 1; DatabaseInstance database_instance = 2; }
//	→ returns "databaseInstance"
//
// Used to auto-derive a `source` extraction on persist cases where the user
// did not set one explicitly, so the wrapper field is unwrapped on the way
// in instead of being merged verbatim into the persisted file. Returns ""
// if there is no matching input field, more than one (ambiguous), or md is
// nil.
func (r *Registry) RequestEntityField(md *desc.MethodDescriptor) string {
	if md == nil {
		return ""
	}
	outType := md.GetOutputType()
	if outType == nil {
		return ""
	}
	inType := md.GetInputType()
	if inType == nil {
		return ""
	}
	var candidate string
	for _, f := range inType.GetFields() {
		if f.IsRepeated() || f.IsMap() {
			continue
		}
		mt := f.GetMessageType()
		if mt == nil {
			continue
		}
		if mt.GetFullyQualifiedName() != outType.GetFullyQualifiedName() {
			continue
		}
		if candidate != "" {
			return "" // ambiguous — more than one matching field
		}
		candidate = f.GetJSONName()
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
