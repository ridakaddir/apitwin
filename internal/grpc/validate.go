package grpc

import "github.com/ridakaddir/apitwin/internal/config"

// schemaAdapter implements config.PersistSchema backed by the proto
// registry. Used by config.ValidateGRPCRoutes during server startup so
// fail-fast validation has access to per-method auto-derive results without
// dragging proto dependencies into the config package.
type schemaAdapter struct {
	reg *Registry
}

// NewSchema returns a config.PersistSchema view of the proto registry. The
// returned schema is read-only and safe for concurrent use.
func NewSchema(reg *Registry) config.PersistSchema {
	return &schemaAdapter{reg: reg}
}

func (s *schemaAdapter) EntityFields(routeMatch, wrap string) (map[string]bool, bool) {
	md, _ := s.reg.FindMethod(routeMatch)
	if md == nil {
		return nil, false
	}
	if wrap == "" {
		return entityFieldsFromType(md.GetOutputType()), true
	}
	set := s.reg.EntityFieldNames(md, wrap)
	return set, set != nil
}

func (s *schemaAdapter) DeriveWrapSource(routeMatch string) (string, string, bool, bool) {
	md, _ := s.reg.FindMethod(routeMatch)
	if md == nil {
		return "", "", false, false
	}
	src, wrap, amb := s.reg.RequestEntity(md)
	return wrap, src, amb, true
}

func (s *schemaAdapter) MultiMessageResponse(routeMatch string) (bool, bool) {
	md, _ := s.reg.FindMethod(routeMatch)
	if md == nil {
		return false, false
	}
	cands := candidateMessageFields(md.GetOutputType())
	return len(cands) >= 2, true
}
