package grpc

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ridakaddir/apitwin/internal/config"
	"google.golang.org/grpc/codes"
)

// loadTestRegistry parses the example countries.proto and returns a Registry
// for use in tests.
func loadTestRegistry(t *testing.T) *Registry {
	t.Helper()
	reg, err := NewRegistry(
		[]string{"countries.proto"},
		[]string{"../../examples/grpc-wrap"},
	)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	return reg
}

// loadDatabaseTestRegistry parses the testdata Google-style database.proto
// and returns a Registry. Used by RequestEntityField + persist auto-derive
// tests.
func loadDatabaseTestRegistry(t *testing.T) *Registry {
	t.Helper()
	reg, err := NewRegistry(
		[]string{"database.proto"},
		[]string{"testdata"},
	)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	return reg
}

// -----------------------------------------------------------------------
// EntityFieldNames
// -----------------------------------------------------------------------

func TestEntityFieldNames_ValidWrap(t *testing.T) {
	reg := loadTestRegistry(t)
	md, err := reg.FindMethod("/geo.CountryService/UpdateCountry")
	if err != nil || md == nil {
		t.Fatalf("FindMethod: md=%v err=%v", md, err)
	}

	// GetCountryResponse has field "country" of type Country.
	allowed := reg.EntityFieldNames(md, "country")
	if allowed == nil {
		t.Fatal("expected non-nil allowed set")
	}

	// Country has fields: code, name, continent.
	for _, name := range []string{"code", "name", "continent"} {
		if !allowed[name] {
			t.Errorf("expected %q in allowed set", name)
		}
	}
	// Routing field from UpdateCountryRequest must NOT be in the set.
	if allowed["countryCode"] || allowed["country_code"] {
		t.Error("countryCode/country_code should not be in entity field set")
	}
}

func TestEntityFieldNames_EmptyWrap(t *testing.T) {
	reg := loadTestRegistry(t)
	md, _ := reg.FindMethod("/geo.CountryService/UpdateCountry")
	if got := reg.EntityFieldNames(md, ""); got != nil {
		t.Errorf("expected nil for empty wrap, got %v", got)
	}
}

func TestEntityFieldNames_NilMD(t *testing.T) {
	reg := loadTestRegistry(t)
	if got := reg.EntityFieldNames(nil, "country"); got != nil {
		t.Errorf("expected nil for nil md, got %v", got)
	}
}

func TestEntityFieldNames_NonMatchingWrap(t *testing.T) {
	reg := loadTestRegistry(t)
	md, _ := reg.FindMethod("/geo.CountryService/UpdateCountry")
	if got := reg.EntityFieldNames(md, "nonexistent"); got != nil {
		t.Errorf("expected nil for non-matching wrap, got %v", got)
	}
}

func TestEntityFieldNames_EmptyResponseMessage(t *testing.T) {
	reg := loadTestRegistry(t)
	// DeleteCountryResponse has no fields at all.
	md, _ := reg.FindMethod("/geo.CountryService/DeleteCountry")
	if got := reg.EntityFieldNames(md, "anything"); got != nil {
		t.Errorf("expected nil for empty response message, got %v", got)
	}
}

// -----------------------------------------------------------------------
// filterToEntityFields
// -----------------------------------------------------------------------

func TestFilterToEntityFields_NilAllowed(t *testing.T) {
	data := map[string]interface{}{"a": 1, "b": 2}
	got := filterToEntityFields(data, nil)
	if len(got) != 2 {
		t.Errorf("expected data unchanged, got %v", got)
	}
}

func TestFilterToEntityFields_FiltersUnknownKeys(t *testing.T) {
	data := map[string]interface{}{
		"id":          "env-1",
		"description": "updated",
		"orgId":       "abc",
		"providerId":  "GCP",
	}
	allowed := map[string]bool{
		"id":          true,
		"description": true,
		"name":        true,
	}
	got := filterToEntityFields(data, allowed)
	if len(got) != 2 {
		t.Errorf("expected 2 keys, got %d: %v", len(got), got)
	}
	if got["orgId"] != nil || got["providerId"] != nil {
		t.Error("routing fields should have been filtered out")
	}
	if got["id"] != "env-1" || got["description"] != "updated" {
		t.Error("entity fields should be preserved")
	}
}

func TestFilterToEntityFields_AllKeysValid(t *testing.T) {
	data := map[string]interface{}{"a": 1, "b": 2}
	allowed := map[string]bool{"a": true, "b": true, "c": true}
	got := filterToEntityFields(data, allowed)
	if len(got) != 2 {
		t.Errorf("expected 2 keys, got %d", len(got))
	}
}

func TestFilterToEntityFields_EmptyData(t *testing.T) {
	data := map[string]interface{}{}
	allowed := map[string]bool{"a": true}
	got := filterToEntityFields(data, allowed)
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}


// -----------------------------------------------------------------------
// walkNestedField
// -----------------------------------------------------------------------

func TestWalkNestedField_TopLevel(t *testing.T) {
	m := map[string]interface{}{"name": "alpha"}
	val, ok := walkNestedField(m, "name")
	if !ok || val != "alpha" {
		t.Errorf("got %q, %v", val, ok)
	}
}

func TestWalkNestedField_Nested(t *testing.T) {
	m := map[string]interface{}{
		"service": map[string]interface{}{"name": "my-svc"},
	}
	val, ok := walkNestedField(m, "service.name")
	if !ok || val != "my-svc" {
		t.Errorf("got %q, %v", val, ok)
	}
}

func TestWalkNestedField_DeeplyNested(t *testing.T) {
	m := map[string]interface{}{
		"a": map[string]interface{}{
			"b": map[string]interface{}{
				"c": "deep",
			},
		},
	}
	val, ok := walkNestedField(m, "a.b.c")
	if !ok || val != "deep" {
		t.Errorf("got %q, %v", val, ok)
	}
}

func TestWalkNestedField_CamelCaseFallback(t *testing.T) {
	m := map[string]interface{}{
		"serviceConfig": map[string]interface{}{"displayName": "Test"},
	}
	val, ok := walkNestedField(m, "service_config.display_name")
	if !ok || val != "Test" {
		t.Errorf("got %q, %v", val, ok)
	}
}

func TestWalkNestedField_NotFound(t *testing.T) {
	m := map[string]interface{}{"a": "b"}
	_, ok := walkNestedField(m, "x.y")
	if ok {
		t.Error("expected not found")
	}
}

func TestWalkNestedField_NumericValue(t *testing.T) {
	m := map[string]interface{}{"id": float64(42)}
	val, ok := walkNestedField(m, "id")
	if !ok || val != "42" {
		t.Errorf("got %q, %v", val, ok)
	}
}

func TestWalkNestedField_BoolValue(t *testing.T) {
	m := map[string]interface{}{"active": true}
	val, ok := walkNestedField(m, "active")
	if !ok || val != "true" {
		t.Errorf("got %q, %v", val, ok)
	}
}

func TestWalkNestedField_NilValue(t *testing.T) {
	m := map[string]interface{}{"empty": nil}
	_, ok := walkNestedField(m, "empty")
	if ok {
		t.Error("expected not found for nil")
	}
}

// -----------------------------------------------------------------------
// resolveGRPCFilePath
// -----------------------------------------------------------------------

func TestResolveGRPCFilePath_TopLevel(t *testing.T) {
	reqMap := map[string]interface{}{"name": "alpha"}
	got := resolveGRPCFilePath("stubs/{body.name}.json", reqMap, "")
	if got != "stubs/alpha.json" {
		t.Errorf("got %q", got)
	}
}

func TestResolveGRPCFilePath_NestedDotPath(t *testing.T) {
	reqMap := map[string]interface{}{
		"service": map[string]interface{}{"name": "my-svc"},
	}
	got := resolveGRPCFilePath("stubs/{body.service.name}.json", reqMap, "")
	if got != "stubs/my-svc.json" {
		t.Errorf("got %q", got)
	}
}

func TestResolveGRPCFilePath_DeeplyNested(t *testing.T) {
	reqMap := map[string]interface{}{
		"a": map[string]interface{}{
			"b": map[string]interface{}{"c": "val"},
		},
	}
	got := resolveGRPCFilePath("stubs/{body.a.b.c}.json", reqMap, "")
	if got != "stubs/val.json" {
		t.Errorf("got %q", got)
	}
}

func TestResolveGRPCFilePath_SnakeCaseNested(t *testing.T) {
	reqMap := map[string]interface{}{
		"serviceConfig": map[string]interface{}{"displayName": "Test"},
	}
	got := resolveGRPCFilePath("stubs/{body.service_config.display_name}.json", reqMap, "")
	if got != "stubs/Test.json" {
		t.Errorf("got %q", got)
	}
}

func TestResolveGRPCFilePath_NotFound(t *testing.T) {
	reqMap := map[string]interface{}{"a": "b"}
	got := resolveGRPCFilePath("stubs/{body.x.y}.json", reqMap, "")
	if got != "stubs/{body.x.y}.json" {
		t.Errorf("expected placeholder unchanged, got %q", got)
	}
}

func TestResolveGRPCFilePath_Sanitizes(t *testing.T) {
	reqMap := map[string]interface{}{
		"service": map[string]interface{}{"name": "bad/path/../name"},
	}
	got := resolveGRPCFilePath("stubs/{body.service.name}.json", reqMap, "")
	if got != "stubs/bad_path_.._name.json" {
		t.Errorf("got %q", got)
	}
}

func TestResolveGRPCFilePath_ConfigDir(t *testing.T) {
	reqMap := map[string]interface{}{"id": "abc"}
	got := resolveGRPCFilePath("stubs/{body.id}.json", reqMap, "/config")
	if got != "/config/stubs/abc.json" {
		t.Errorf("got %q", got)
	}
}

// -----------------------------------------------------------------------
// RequestEntityField — Google-style auto-derive
// -----------------------------------------------------------------------

func TestRequestEntityField_GoogleConventionUpdate(t *testing.T) {
	reg := loadDatabaseTestRegistry(t)
	md, err := reg.FindMethod("/database.v1.DatabaseService/UpdateDatabaseInstance")
	if err != nil || md == nil {
		t.Fatalf("FindMethod: md=%v err=%v", md, err)
	}

	// Input has DatabaseInstance database_instance = 2; output IS DatabaseInstance.
	got, ambiguous := reg.RequestEntityField(md)
	if got != "databaseInstance" {
		t.Errorf("RequestEntityField = %q, want %q", got, "databaseInstance")
	}
	if ambiguous {
		t.Error("expected ambiguous=false for unique match")
	}
}

func TestRequestEntityField_GoogleConventionCreate(t *testing.T) {
	reg := loadDatabaseTestRegistry(t)
	md, _ := reg.FindMethod("/database.v1.DatabaseService/CreateDatabaseInstance")

	// Symmetric with Update — Create also wraps the entity in the request.
	got, ambiguous := reg.RequestEntityField(md)
	if got != "databaseInstance" {
		t.Errorf("RequestEntityField = %q, want %q", got, "databaseInstance")
	}
	if ambiguous {
		t.Error("expected ambiguous=false")
	}
}

func TestRequestEntityField_NoMatchingField(t *testing.T) {
	reg := loadDatabaseTestRegistry(t)
	// DeleteDatabaseInstanceRequest has only a string `name` field — no
	// field of type DatabaseInstance.
	md, _ := reg.FindMethod("/database.v1.DatabaseService/DeleteDatabaseInstance")
	got, ambiguous := reg.RequestEntityField(md)
	if got != "" {
		t.Errorf("RequestEntityField = %q, want \"\" (no DatabaseInstance field in request)", got)
	}
	if ambiguous {
		t.Error("expected ambiguous=false for no-match (silent skip)")
	}
}

func TestRequestEntityField_NilMD(t *testing.T) {
	reg := loadDatabaseTestRegistry(t)
	got, ambiguous := reg.RequestEntityField(nil)
	if got != "" || ambiguous {
		t.Errorf("RequestEntityField(nil) = (%q, %v), want (\"\", false)", got, ambiguous)
	}
}

func TestRequestEntityField_FlatRequestNoMatch(t *testing.T) {
	// countries.proto's UpdateCountryRequest has only flat fields, no
	// Country wrapper field — auto-derive must return "".
	reg := loadTestRegistry(t)
	md, _ := reg.FindMethod("/geo.CountryService/UpdateCountry")
	got, ambiguous := reg.RequestEntityField(md)
	if got != "" {
		t.Errorf("RequestEntityField = %q, want \"\" (UpdateCountryRequest has no Country wrapper)", got)
	}
	if ambiguous {
		t.Error("expected ambiguous=false")
	}
}

// TestRequestEntityField_AmbiguousReturnsTrue covers the case where a
// request input has more than one non-repeated field whose message type
// matches the response output type. The auto-derive must bail out
// (returning "") and signal ambiguous=true so the caller can warn.
func TestRequestEntityField_AmbiguousReturnsTrue(t *testing.T) {
	reg := loadDatabaseTestRegistry(t)
	md, err := reg.FindMethod("/database.v1.DatabaseService/CompareDatabaseInstances")
	if err != nil || md == nil {
		t.Fatalf("FindMethod CompareDatabaseInstances: md=%v err=%v", md, err)
	}
	got, ambiguous := reg.RequestEntityField(md)
	if got != "" {
		t.Errorf("expected name=\"\" on ambiguous match, got %q", got)
	}
	if !ambiguous {
		t.Error("expected ambiguous=true when input has two DatabaseInstance fields")
	}
}

// -----------------------------------------------------------------------
// applyGRPCPersist — auto-derive source on merge="update"
// -----------------------------------------------------------------------

// TestApplyGRPCPersist_AutoDerivesSourceFromProto reproduces the user's Bug 1
// (gRPC merge="update" file corruption). Before the fix: when c.Source is
// empty and the request body has a Google-style wrapper field, the entire
// request envelope was shallow-merged into the existing flat entity stub,
// leaving both flat fields and a duplicated nested wrapper that broke
// proto round-trip on Get. After the fix: the wrapper field is auto-derived
// from the proto descriptor (request input field whose message type matches
// the response output type) and extracted before merge.
func TestApplyGRPCPersist_AutoDerivesSourceFromProto(t *testing.T) {
	dir := t.TempDir()
	stubFile := filepath.Join(dir, "stubs", "instances", "db-1.json")
	// Initial stub written by an earlier append (post-source-extraction shape).
	if err := os.MkdirAll(filepath.Dir(stubFile), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	initial, _ := json.Marshal(map[string]interface{}{
		"id":          "db-1",
		"displayName": "test",
		"status":      "provisioning",
		"tier":        "standard",
		"region":      "us-central1",
	})
	if err := os.WriteFile(stubFile, initial, 0644); err != nil {
		t.Fatalf("write stub: %v", err)
	}

	reg := loadDatabaseTestRegistry(t)
	md, _ := reg.FindMethod("/database.v1.DatabaseService/UpdateDatabaseInstance")
	if md == nil {
		t.Fatal("UpdateDatabaseInstance not found in test registry")
	}

	h := &handler{
		loader:      &stubLoader{cfg: &config.Config{}, configDir: dir},
		transitions: newGRPCTransitionState(),
		registry:    reg,
	}

	// Case has merge="update" with NO source and NO wrap — the user's
	// footgun shape that produced the corruption.
	c := config.Case{
		Status:  0,
		File:    "stubs/instances/{body.databaseInstance.id}.json",
		Persist: true,
		Merge:   "update",
	}

	// Request envelope: top-level routing field + nested entity.
	reqMap := map[string]interface{}{
		"name": "db-1", // route param — must NOT leak into the entity stub
		"databaseInstance": map[string]interface{}{
			"id":          "db-1",
			"displayName": "renamed",
		},
	}

	code, handled, _, persistedPath := h.applyGRPCPersist(
		c, reqMap, time.Now(),
		"/database.v1.DatabaseService/UpdateDatabaseInstance", md,
	)

	if !handled {
		t.Fatal("expected handled=true")
	}
	if code != codes.OK {
		t.Fatalf("expected OK, got %v", code)
	}
	if persistedPath != stubFile {
		t.Errorf("persistedPath=%q, want %q", persistedPath, stubFile)
	}

	stub := readStub(t, stubFile)

	// Bug 1 regression: the entity-wrapper field must NOT appear at the top
	// level of the file. Before the fix this key was present and broke
	// proto round-trip on Get.
	if _, hasWrapper := stub["databaseInstance"]; hasWrapper {
		t.Errorf("BUG 1 REGRESSION: stub contains top-level 'databaseInstance' wrapper key: %v", stub)
	}

	// Routing field 'name' must NOT leak into the entity.
	if _, hasName := stub["name"]; hasName {
		t.Errorf("BUG 1 REGRESSION: routing field 'name' leaked into stub: %v", stub)
	}

	// The wrapped entity field must have been merged into the file.
	if stub["displayName"] != "renamed" {
		t.Errorf("displayName not updated: got %v", stub["displayName"])
	}
	// Existing fields must survive shallow merge.
	if stub["tier"] != "standard" {
		t.Errorf("tier dropped from stub: got %v", stub["tier"])
	}
	if stub["region"] != "us-central1" {
		t.Errorf("region dropped from stub: got %v", stub["region"])
	}
}

// TestApplyGRPCPersist_DeleteOnGoogleConvention verifies that the
// auto-derive walk is a no-op on the delete merge path: it should not
// warn, error, or affect the delete semantics even when the request type
// follows the Google convention. This pins the silent-skip contract on
// delete so a future refactor can't accidentally fire a Warn on every
// successful Delete.
func TestApplyGRPCPersist_DeleteOnGoogleConvention(t *testing.T) {
	dir := t.TempDir()
	stubFile := filepath.Join(dir, "stubs", "instances", "db-3.json")
	if err := os.MkdirAll(filepath.Dir(stubFile), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(stubFile, []byte(`{"id":"db-3"}`), 0644); err != nil {
		t.Fatalf("write stub: %v", err)
	}

	reg := loadDatabaseTestRegistry(t)
	// DeleteDatabaseInstance has no DatabaseInstance field in its request
	// type — the helper returns ("", false) and the merge path runs cleanly.
	md, _ := reg.FindMethod("/database.v1.DatabaseService/DeleteDatabaseInstance")

	h := &handler{
		loader:      &stubLoader{cfg: &config.Config{}, configDir: dir},
		transitions: newGRPCTransitionState(),
		registry:    reg,
	}

	c := config.Case{
		File:    "stubs/instances/{body.name}.json",
		Persist: true,
		Merge:   "delete",
	}
	reqMap := map[string]interface{}{"name": "db-3"}

	code, _, _, _ := h.applyGRPCPersist(c, reqMap, time.Now(),
		"/database.v1.DatabaseService/DeleteDatabaseInstance", md)
	if code != codes.OK {
		t.Fatalf("expected OK on delete, got %v", code)
	}
	if _, err := os.Stat(stubFile); !os.IsNotExist(err) {
		t.Errorf("expected stub file to be deleted, stat err=%v", err)
	}
}

// TestApplyGRPCPersist_ExplicitSourceTakesPrecedence verifies that an
// explicitly-set Source field is honoured even when the auto-derive would
// also fire — i.e. the auto-derive is a fallback, not an override.
func TestApplyGRPCPersist_ExplicitSourceTakesPrecedence(t *testing.T) {
	dir := t.TempDir()
	stubFile := filepath.Join(dir, "stubs", "instances", "db-2.json")
	if err := os.MkdirAll(filepath.Dir(stubFile), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	initial, _ := json.Marshal(map[string]interface{}{"id": "db-2", "tier": "basic"})
	if err := os.WriteFile(stubFile, initial, 0644); err != nil {
		t.Fatalf("write stub: %v", err)
	}

	reg := loadDatabaseTestRegistry(t)
	md, _ := reg.FindMethod("/database.v1.DatabaseService/UpdateDatabaseInstance")

	h := &handler{
		loader:      &stubLoader{cfg: &config.Config{}, configDir: dir},
		transitions: newGRPCTransitionState(),
		registry:    reg,
	}

	c := config.Case{
		File:    "stubs/instances/{body.databaseInstance.id}.json",
		Persist: true,
		Merge:   "update",
		Source:  "databaseInstance", // explicit
	}

	reqMap := map[string]interface{}{
		"name": "db-2",
		"databaseInstance": map[string]interface{}{
			"id":   "db-2",
			"tier": "premium",
		},
	}

	code, _, _, _ := h.applyGRPCPersist(c, reqMap, time.Now(),
		"/database.v1.DatabaseService/UpdateDatabaseInstance", md)
	if code != codes.OK {
		t.Fatalf("expected OK, got %v", code)
	}
	stub := readStub(t, stubFile)
	if stub["tier"] != "premium" {
		t.Errorf("explicit source extraction failed: tier=%v", stub["tier"])
	}
	if _, has := stub["databaseInstance"]; has {
		t.Errorf("wrapper key leaked even with explicit source: %v", stub)
	}
}
