package config

// Config is the top-level structure for apitwin config files (JSON/YAML/TOML).
type Config struct {
	Routes     []Route     `json:"routes,omitempty"      yaml:"routes,omitempty"      toml:"routes,omitempty"`
	GRPCRoutes []GRPCRoute `json:"grpc_routes,omitempty" yaml:"grpc_routes,omitempty" toml:"grpc_routes,omitempty"`
}

// GRPCRoute defines a single interceptable gRPC method.
// Match is the full gRPC method path: "/package.Service/Method".
// Cases reuse the same Case struct; Case.Status maps to a gRPC status code
// (0=OK, 1=CANCELLED, 2=UNKNOWN, 3=INVALID_ARGUMENT, 4=DEADLINE_EXCEEDED,
//
//	5=NOT_FOUND, 6=ALREADY_EXISTS, 7=PERMISSION_DENIED, 13=INTERNAL, 14=UNAVAILABLE).
//
// Case.File and Case.JSON hold protojson-compatible JSON (field names match the proto field names).
type GRPCRoute struct {
	Match       string          `json:"match"                          yaml:"match"                          toml:"match"`
	Enabled     *bool           `json:"enabled,omitempty"              yaml:"enabled,omitempty"              toml:"enabled,omitempty"`
	Fallback    string          `json:"fallback,omitempty"             yaml:"fallback,omitempty"             toml:"fallback,omitempty"`
	Conditions  []Condition     `json:"conditions,omitempty"           yaml:"conditions,omitempty"           toml:"conditions,omitempty"`
	Cases       map[string]Case `json:"cases"                          yaml:"cases"                          toml:"cases"`
	Transitions []Transition    `json:"transitions,omitempty"          yaml:"transitions,omitempty"          toml:"transitions,omitempty"`
}

// IsEnabled returns true if the gRPC route is enabled (defaults to true if not set).
func (r *GRPCRoute) IsEnabled() bool {
	if r.Enabled == nil {
		return true
	}
	return *r.Enabled
}

// GetTransitions returns the route's transition sequence. Satisfies the
// transitions.RouteLike interface so gRPC routes can drive the shared
// state machine.
func (r *GRPCRoute) GetTransitions() []Transition {
	return r.Transitions
}

// Route defines a single interceptable HTTP endpoint.
type Route struct {
	Method      string          `json:"method"                yaml:"method"                toml:"method"`
	Match       string          `json:"match"                 yaml:"match"                 toml:"match"`
	Enabled     *bool           `json:"enabled,omitempty"     yaml:"enabled,omitempty"     toml:"enabled,omitempty"`
	Fallback    string          `json:"fallback,omitempty"    yaml:"fallback,omitempty"    toml:"fallback,omitempty"`
	Conditions  []Condition     `json:"conditions,omitempty"  yaml:"conditions,omitempty"  toml:"conditions,omitempty"`
	Cases       map[string]Case `json:"cases"                 yaml:"cases"                 toml:"cases"`
	Transitions []Transition    `json:"transitions,omitempty" yaml:"transitions,omitempty" toml:"transitions,omitempty"`
}

// Transition defines one step in a time-based response sequence.
type Transition struct {
	Case     string `json:"case"               yaml:"case"               toml:"case"`
	Duration int    `json:"duration,omitempty" yaml:"duration,omitempty" toml:"duration,omitempty"` // seconds this state lasts; omit or 0 on the last entry for a terminal state
}

// IsEnabled returns true if the route is enabled (defaults to true if not set).
func (r *Route) IsEnabled() bool {
	if r.Enabled == nil {
		return true
	}
	return *r.Enabled
}

// GetTransitions returns the route's transition sequence. Satisfies the
// transitions.RouteLike interface so REST routes can drive the shared
// state machine.
func (r *Route) GetTransitions() []Transition {
	return r.Transitions
}

// Condition maps an incoming request attribute to a case name.
type Condition struct {
	Source string `json:"source"          yaml:"source"          toml:"source"`          // body | query | header
	Field  string `json:"field"           yaml:"field"           toml:"field"`           // dot-notation or key name
	Op     string `json:"op"              yaml:"op"              toml:"op"`              // eq | neq | contains | regex | exists | not_exists
	Value  string `json:"value,omitempty" yaml:"value,omitempty" toml:"value,omitempty"` // unused for exists / not_exists
	Case   string `json:"case"            yaml:"case"            toml:"case"`            // case key to activate
}

// Case defines a mock response.
type Case struct {
	Status   int    `json:"status,omitempty"    yaml:"status,omitempty"    toml:"status,omitempty"`
	JSON     string `json:"json,omitempty"      yaml:"json,omitempty"      toml:"json,omitempty"`
	File     string `json:"file,omitempty"      yaml:"file,omitempty"      toml:"file,omitempty"`
	Delay    int    `json:"delay,omitempty"     yaml:"delay,omitempty"     toml:"delay,omitempty"`
	Persist  bool   `json:"persist,omitempty"   yaml:"persist,omitempty"   toml:"persist,omitempty"`
	Merge    string `json:"merge,omitempty"     yaml:"merge,omitempty"     toml:"merge,omitempty"`     // append | update | delete | cascade
	Key      string `json:"key,omitempty"       yaml:"key,omitempty"       toml:"key,omitempty"`       // record lookup key
	ArrayKey string `json:"array_key,omitempty" yaml:"array_key,omitempty" toml:"array_key,omitempty"` // array field in stub JSON
	Defaults string `json:"defaults,omitempty"  yaml:"defaults,omitempty"  toml:"defaults,omitempty"`  // JSON file with default values for append/update
	Wrap     string `json:"wrap,omitempty"      yaml:"wrap,omitempty"      toml:"wrap,omitempty"`      // wrap response into {"field": <content>}
	Source   string `json:"source,omitempty"    yaml:"source,omitempty"    toml:"source,omitempty"`    // dot-path into request body; only that sub-object is persisted

	// Cascade mutation fields
	Primary *CascadePrimary `json:"primary,omitempty"  yaml:"primary,omitempty"  toml:"primary,omitempty"`
	Cascade []CascadeTarget `json:"cascade,omitempty"  yaml:"cascade,omitempty"  toml:"cascade,omitempty"`
}

// CascadePrimary defines the primary file operation in a cascade mutation.
type CascadePrimary struct {
	File  string `json:"file"           yaml:"file"           toml:"file"`
	Merge string `json:"merge"          yaml:"merge"          toml:"merge"`          // update | append | delete
	Path  string `json:"path,omitempty" yaml:"path,omitempty" toml:"path,omitempty"` // optional field path for targeted updates
}

// CascadeTarget defines a cascade target file operation.
type CascadeTarget struct {
	Pattern   string `json:"pattern"             yaml:"pattern"             toml:"pattern"`             // file pattern (supports wildcards)
	Merge     string `json:"merge"               yaml:"merge"               toml:"merge"`               // update | append | delete
	Path      string `json:"path,omitempty"      yaml:"path,omitempty"      toml:"path,omitempty"`      // optional field path for targeted updates
	Transform string `json:"transform,omitempty" yaml:"transform,omitempty" toml:"transform,omitempty"` // JSONPath expression for data transformation
	Condition string `json:"condition,omitempty" yaml:"condition,omitempty" toml:"condition,omitempty"` // optional condition for cascade execution
}

// StatusCode returns the HTTP status for a case, defaulting to 200.
func (c *Case) StatusCode() int {
	if c.Status == 0 {
		return 200
	}
	return c.Status
}
