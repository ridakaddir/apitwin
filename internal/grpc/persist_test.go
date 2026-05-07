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

// loadGeoTestRegistry parses the testdata geo.proto and returns a Registry.
// Used by RequestEntity tests that exercise the response-wrapped RPC shape
// reported against UpdateGateway (see Registry.RequestEntity doc comment).
func loadGeoTestRegistry(t *testing.T) *Registry {
	t.Helper()
	reg, err := NewRegistry(
		[]string{"geo.proto"},
		[]string{"testdata"},
	)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	return reg
}

// loadLROTestRegistry parses the testdata lro.proto and returns a Registry.
// Used by RequestEntity Shape 3 tests — multi-message responses where the
// entity must be picked via request-side correlation (the user's
// UpdateDatabaseInstance shape).
func loadLROTestRegistry(t *testing.T) *Registry {
	t.Helper()
	reg, err := NewRegistry(
		[]string{"lro.proto"},
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

// -----------------------------------------------------------------------
// RequestEntity — two-shape auto-derive (direct + response-wrapped)
// -----------------------------------------------------------------------

// TestRequestEntity_DirectEntityResponse covers shape 1: response IS the
// entity. PatchCity returns City directly, so the auto-derive picks the
// request's `city` field and leaves wrap empty (no wrapping needed to
// encode a City).
func TestRequestEntity_DirectEntityResponse(t *testing.T) {
	reg := loadGeoTestRegistry(t)
	md, err := reg.FindMethod("/geo.v1.CityService/PatchCity")
	if err != nil || md == nil {
		t.Fatalf("FindMethod PatchCity: md=%v err=%v", md, err)
	}
	src, wrap, amb := reg.RequestEntity(md)
	if amb {
		t.Error("expected ambiguous=false")
	}
	if src != "city" {
		t.Errorf("source = %q, want %q", src, "city")
	}
	if wrap != "" {
		t.Errorf("wrap = %q, want empty (response IS the entity)", wrap)
	}
}

// TestRequestEntity_ResponseWrapped covers shape 2: UpdateCity returns
// UpdateCityResponse, which wraps a single City field. Auto-derive must
// unwrap the response to City, find the request's `city` field, and
// report both source="city" and wrap="city".
//
// This is the exact shape that produced the reported UpdateGateway
// corruption on beta.12.
func TestRequestEntity_ResponseWrapped(t *testing.T) {
	reg := loadGeoTestRegistry(t)
	md, err := reg.FindMethod("/geo.v1.CityService/UpdateCity")
	if err != nil || md == nil {
		t.Fatalf("FindMethod UpdateCity: md=%v err=%v", md, err)
	}
	src, wrap, amb := reg.RequestEntity(md)
	if amb {
		t.Error("expected ambiguous=false on clean wrapper shape")
	}
	if src != "city" {
		t.Errorf("source = %q, want %q (city field on UpdateCityRequest)", src, "city")
	}
	if wrap != "city" {
		t.Errorf("wrap = %q, want %q (city field on UpdateCityResponse)", wrap, "city")
	}
}

// TestRequestEntity_ResponseWrappedAmbiguous covers shape 2 ambiguity:
// MergeCity's request has two non-repeated City fields, so auto-derive
// must bail out and signal ambiguity instead of guessing.
func TestRequestEntity_ResponseWrappedAmbiguous(t *testing.T) {
	reg := loadGeoTestRegistry(t)
	md, err := reg.FindMethod("/geo.v1.CityService/MergeCity")
	if err != nil || md == nil {
		t.Fatalf("FindMethod MergeCity: md=%v err=%v", md, err)
	}
	src, wrap, amb := reg.RequestEntity(md)
	if !amb {
		t.Error("expected ambiguous=true when request has two City fields")
	}
	if src != "" || wrap != "" {
		t.Errorf("RequestEntity = (%q, %q), want (\"\", \"\") on ambiguous match", src, wrap)
	}
}

// TestRequestEntity_ScalarPrefixDoesNotMatch pins the regression guard
// the user asked for: a scalar field whose camelCase name shares a prefix
// with the entity field (here `city_name` → `cityName` alongside the
// entity field `city`) must NOT cause auto-derive to mis-fire on the
// response-wrapped shape. Scalars are filtered out by type before the
// name is ever consulted.
func TestRequestEntity_ScalarPrefixDoesNotMatch(t *testing.T) {
	reg := loadGeoTestRegistry(t)
	md, _ := reg.FindMethod("/geo.v1.CityService/UpdateCity")
	// UpdateCityRequest has: parent (CityParent), city_name (string),
	// city (City). The scalar city_name must be invisible to auto-derive
	// — source/wrap must still resolve to ("city", "city", false) cleanly.
	src, wrap, amb := reg.RequestEntity(md)
	if amb || src != "city" || wrap != "city" {
		t.Errorf("scalar prefix collision broke auto-derive: got (%q, %q, %v)", src, wrap, amb)
	}
}

// TestRequestEntityField_BackwardsCompatShim verifies the legacy
// RequestEntityField wrapper still returns (source, ambiguous) for the
// shape-1 case without leaking the new wrap return value. Existing
// callers of RequestEntityField must keep compiling.
func TestRequestEntityField_BackwardsCompatShim(t *testing.T) {
	reg := loadDatabaseTestRegistry(t)
	md, _ := reg.FindMethod("/database.v1.DatabaseService/UpdateDatabaseInstance")
	src, amb := reg.RequestEntityField(md)
	if amb {
		t.Error("expected ambiguous=false")
	}
	if src != "databaseInstance" {
		t.Errorf("RequestEntityField = %q, want %q", src, "databaseInstance")
	}
}

// -----------------------------------------------------------------------
// applyGRPCPersist — auto-derive of source AND wrap on response-wrapped shape
// -----------------------------------------------------------------------

// TestApplyGRPCPersist_AutoDerivesSourceAndWrapOnResponseWrapper is the
// end-to-end regression test for the UpdateGateway bug. The case has NO
// explicit source or wrap (matching the user's `ready` transition case),
// and the proto uses the response-wrapped shape
// (UpdateCityResponse { City city = 1; }).
//
// Before the fix: auto-derive returned "" because no request field had
// type UpdateCityResponse, so the full request envelope (parent, cityName,
// nested city) was merged into the flat existing stub, then returned
// unwrapped from applyGRPCPersist, which would later fail EncodeResponse
// with "message type UpdateCityResponse has no known field named country".
//
// After the fix: RequestEntity unwraps UpdateCityResponse → City, matches
// the request's `city` field, infers both source="city" and wrap="city",
// producing a clean merge and a wrapped response.
func TestApplyGRPCPersist_AutoDerivesSourceAndWrapOnResponseWrapper(t *testing.T) {
	dir := t.TempDir()
	stubFile := filepath.Join(dir, "stubs", "cities", "marrakech.json")
	if err := os.MkdirAll(filepath.Dir(stubFile), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	initial, _ := json.Marshal(map[string]interface{}{
		"name":       "marrakech",
		"country":    "MA",
		"population": float64(928850),
		"status":     "ready",
	})
	if err := os.WriteFile(stubFile, initial, 0644); err != nil {
		t.Fatalf("write stub: %v", err)
	}

	reg := loadGeoTestRegistry(t)
	md, _ := reg.FindMethod("/geo.v1.CityService/UpdateCity")
	if md == nil {
		t.Fatal("UpdateCity not found in geo test registry")
	}

	h := &handler{
		loader:      &stubLoader{cfg: &config.Config{}, configDir: dir},
		transitions: newGRPCTransitionState(),
		registry:    reg,
	}

	// No explicit source / wrap — mirrors the user's `ready` case.
	c := config.Case{
		Status:  0,
		File:    "stubs/cities/{body.city.name}.json",
		Persist: true,
		Merge:   "update",
	}

	reqMap := map[string]interface{}{
		"parent": map[string]interface{}{
			"continent": "africa",
			"country":   "MA",
		},
		"cityName": "marrakech", // prefix-colliding scalar; must not leak
		"city": map[string]interface{}{
			"name":       "marrakech",
			"population": float64(950000),
		},
	}

	code, handled, result, persistedPath := h.applyGRPCPersist(
		c, reqMap, time.Now(),
		"/geo.v1.CityService/UpdateCity", md,
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

	// Response must be wrapped under "city" so EncodeResponse can round-trip
	// it as UpdateCityResponse. This is the bit the caller-side encode
	// failure hinged on.
	inner, ok := result["city"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected result wrapped as {city: ...}, got %v", result)
	}
	if inner["population"] != float64(950000) {
		t.Errorf("wrapped response population = %v, want 950000", inner["population"])
	}

	stub := readStub(t, stubFile)

	// Corruption guard: none of the request envelope keys may appear in
	// the persisted file.
	for _, key := range []string{"parent", "cityName", "city"} {
		if _, has := stub[key]; has {
			t.Errorf("BUG REGRESSION: stub contains leaked key %q: %v", key, stub)
		}
	}

	// Population updated, country/status preserved from seed.
	if stub["population"] != float64(950000) {
		t.Errorf("population not updated: got %v", stub["population"])
	}
	if stub["country"] != "MA" {
		t.Errorf("country dropped: got %v", stub["country"])
	}
	if stub["status"] != "ready" {
		t.Errorf("status dropped: got %v", stub["status"])
	}
}

// TestApplyGRPCPersist_ExplicitSourceShortCircuitsAutoDerive pins the
// contract the user asked for in the bug report: when `source` is set
// explicitly, auto-derive is never consulted. We use the ambiguous
// MergeCity proto here — if auto-derive were called it would warn
// (ambiguous=true) and leave source empty; with explicit source="left"
// the explicit value must win regardless.
func TestApplyGRPCPersist_ExplicitSourceShortCircuitsAutoDerive(t *testing.T) {
	dir := t.TempDir()
	stubFile := filepath.Join(dir, "stubs", "cities", "casablanca.json")
	if err := os.MkdirAll(filepath.Dir(stubFile), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	initial, _ := json.Marshal(map[string]interface{}{
		"name":       "casablanca",
		"country":    "MA",
		"population": float64(3360000),
	})
	if err := os.WriteFile(stubFile, initial, 0644); err != nil {
		t.Fatalf("write stub: %v", err)
	}

	reg := loadGeoTestRegistry(t)
	md, _ := reg.FindMethod("/geo.v1.CityService/MergeCity")

	h := &handler{
		loader:      &stubLoader{cfg: &config.Config{}, configDir: dir},
		transitions: newGRPCTransitionState(),
		registry:    reg,
	}

	c := config.Case{
		Status:  0,
		File:    "stubs/cities/{body.left.name}.json",
		Persist: true,
		Merge:   "update",
		Source:  "left", // explicit — auto-derive would have said ambiguous
		Wrap:    "city", // explicit — matches MergeCity's UpdateCityResponse wrapper
	}

	reqMap := map[string]interface{}{
		"left": map[string]interface{}{
			"name":       "casablanca",
			"population": float64(3400000),
		},
		"right": map[string]interface{}{
			"name": "casablanca",
		},
	}

	code, handled, result, _ := h.applyGRPCPersist(
		c, reqMap, time.Now(),
		"/geo.v1.CityService/MergeCity", md,
	)
	if !handled || code != codes.OK {
		t.Fatalf("expected handled=true, OK; got handled=%v, code=%v", handled, code)
	}

	// Response wrapped under explicit wrap="city".
	if _, ok := result["city"]; !ok {
		t.Errorf("expected result wrapped as {city: ...}, got %v", result)
	}

	stub := readStub(t, stubFile)
	if stub["population"] != float64(3400000) {
		t.Errorf("explicit source=left did not extract: population=%v", stub["population"])
	}
	if _, has := stub["right"]; has {
		t.Errorf("right leaked even with explicit source=left: %v", stub)
	}
	if _, has := stub["left"]; has {
		t.Errorf("left wrapper leaked: %v", stub)
	}
}

// TestApplyGRPCPersist_ExplicitSourceInfersWrapFromResponse is the regression
// test for the UpdateSummit bug: when the user sets an explicit `source` but
// leaves `wrap` empty, the handler must still infer the wrap from the
// response descriptor. Source extraction and response wrapping are
// independent concerns — making source explicit should not silently disable
// wrap inference.
//
// Before the fix: `if sourceField == ""` short-circuited the whole
// auto-derive block, so wrap stayed empty. The persist result was returned
// unwrapped and downstream EncodeResponse failed with
// "message type UpdateCityResponse has no known field named country".
//
// After the fix: explicit source branches into a wrap-only inference via
// Registry.ResponseWrap, which returns "city" for UpdateCity's
// UpdateCityResponse wrapper.
func TestApplyGRPCPersist_ExplicitSourceInfersWrapFromResponse(t *testing.T) {
	dir := t.TempDir()
	stubFile := filepath.Join(dir, "stubs", "cities", "marrakech.json")
	if err := os.MkdirAll(filepath.Dir(stubFile), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	initial, _ := json.Marshal(map[string]interface{}{
		"name":       "marrakech",
		"country":    "MA",
		"population": float64(928850),
	})
	if err := os.WriteFile(stubFile, initial, 0644); err != nil {
		t.Fatalf("write stub: %v", err)
	}

	reg := loadGeoTestRegistry(t)
	md, _ := reg.FindMethod("/geo.v1.CityService/UpdateCity")
	if md == nil {
		t.Fatal("UpdateCity not found in geo test registry")
	}

	h := &handler{
		loader:      &stubLoader{cfg: &config.Config{}, configDir: dir},
		transitions: newGRPCTransitionState(),
		registry:    reg,
	}

	// Explicit source, NO explicit wrap — this is the bug shape.
	c := config.Case{
		Status:  0,
		File:    "stubs/cities/{body.city.name}.json",
		Persist: true,
		Merge:   "update",
		Source:  "city",
		// Wrap: "" — must be inferred from UpdateCityResponse.
	}

	reqMap := map[string]interface{}{
		"parent": map[string]interface{}{
			"continent": "africa",
			"country":   "MA",
		},
		"cityName": "marrakech",
		"city": map[string]interface{}{
			"name":       "marrakech",
			"population": float64(950000),
		},
	}

	code, handled, result, _ := h.applyGRPCPersist(
		c, reqMap, time.Now(),
		"/geo.v1.CityService/UpdateCity", md,
	)
	if !handled || code != codes.OK {
		t.Fatalf("expected handled=true, OK; got handled=%v, code=%v", handled, code)
	}

	// The fix: result must be wrapped under the inferred "city" envelope so
	// EncodeResponse can round-trip it through UpdateCityResponse.
	inner, ok := result["city"].(map[string]interface{})
	if !ok {
		t.Fatalf("BUG REGRESSION: expected result wrapped as {city: ...} via inferred wrap, got %v", result)
	}
	if inner["population"] != float64(950000) {
		t.Errorf("wrapped response population = %v, want 950000", inner["population"])
	}
}

// -----------------------------------------------------------------------
// Shape 3 — LRO multi-message responses
// -----------------------------------------------------------------------

// TestRequestEntity_LROResponseShapeCorrelatesViaRequest pins Shape 3: when
// the response has multiple non-repeated message fields (so Shape 2 bails
// out as "multi-candidate"), RequestEntity must correlate against the
// request input and pick the field whose type also appears as a unique
// request field.
//
// This is the user's reported bug shape: UpdateCountryResponse has
// {region_id, charter_id, locale, country, audit}; only Country appears in
// the request input as a message-typed field, so the auto-derive returns
// ("country", "country", false).
func TestRequestEntity_LROResponseShapeCorrelatesViaRequest(t *testing.T) {
	reg := loadLROTestRegistry(t)
	md, err := reg.FindMethod("/lro.v1.CountryService/UpdateCountry")
	if err != nil || md == nil {
		t.Fatalf("FindMethod UpdateCountry: md=%v err=%v", md, err)
	}
	src, wrap, amb := reg.RequestEntity(md)
	if amb {
		t.Error("expected ambiguous=false on LRO shape with one correlation")
	}
	if src != "country" {
		t.Errorf("source = %q, want %q", src, "country")
	}
	if wrap != "country" {
		t.Errorf("wrap = %q, want %q", wrap, "country")
	}
}

// TestRequestEntity_LROResponseTwoCorrelatedFieldsAmbiguous covers the
// Shape 3 ambiguity guard: when both response message fields have types
// that each appear as a unique request field, auto-derive must bail out
// rather than guess.
func TestRequestEntity_LROResponseTwoCorrelatedFieldsAmbiguous(t *testing.T) {
	reg := loadLROTestRegistry(t)
	md, err := reg.FindMethod("/lro.v1.CountryService/MergeCountries")
	if err != nil || md == nil {
		t.Fatalf("FindMethod MergeCountries: md=%v err=%v", md, err)
	}
	src, wrap, amb := reg.RequestEntity(md)
	if !amb {
		t.Error("expected ambiguous=true when two response message fields each correlate")
	}
	if src != "" || wrap != "" {
		t.Errorf("RequestEntity = (%q, %q), want (\"\", \"\")", src, wrap)
	}
}

// TestUniqueNonRepeatedMessageField_SkipsScalars pins the relaxation: a
// response message with a mix of scalars and a single message field must
// resolve to that message field. Pre-fix this returned nil because the
// first scalar caused an early bail-out.
func TestUniqueNonRepeatedMessageField_SkipsScalars(t *testing.T) {
	// UpdateCountryResponse has 3 scalars + 2 message fields. Multiple
	// message fields means uniqueNonRepeatedMessageField still returns nil
	// — but must not bail on the leading scalars; verified indirectly via
	// candidateMessageFields below.
	reg := loadLROTestRegistry(t)
	md, _ := reg.FindMethod("/lro.v1.CountryService/UpdateCountry")
	out := md.GetOutputType()
	if got := uniqueNonRepeatedMessageField(out); got != nil {
		t.Errorf("uniqueNonRepeatedMessageField(UpdateCountryResponse) = %v, want nil (multi-candidate)", got)
	}
	cands := candidateMessageFields(out)
	if len(cands) != 2 {
		t.Errorf("candidateMessageFields = %d, want 2", len(cands))
	}
}

// TestResponseWrapForSource_PicksFieldMatchingSource pins the explicit-
// source / implicit-wrap path on multi-message responses: when c.Source is
// set but c.Wrap is empty, the wrap is computed from the response field
// whose type matches the request field at sourcePath.
func TestResponseWrapForSource_PicksFieldMatchingSource(t *testing.T) {
	reg := loadLROTestRegistry(t)
	md, _ := reg.FindMethod("/lro.v1.CountryService/UpdateCountry")

	// sourcePath="country" → request field of type Country → response field
	// of type Country is "country".
	if got := reg.ResponseWrapForSource(md, "country"); got != "country" {
		t.Errorf("ResponseWrapForSource(country) = %q, want %q", got, "country")
	}

	// Unknown source path returns "".
	if got := reg.ResponseWrapForSource(md, "nonexistent"); got != "" {
		t.Errorf("ResponseWrapForSource(nonexistent) = %q, want \"\"", got)
	}

	// Empty source path returns "".
	if got := reg.ResponseWrapForSource(md, ""); got != "" {
		t.Errorf("ResponseWrapForSource(\"\") = %q, want \"\"", got)
	}

	// Scalar source path (locale is a string) returns "".
	if got := reg.ResponseWrapForSource(md, "locale"); got != "" {
		t.Errorf("ResponseWrapForSource(locale) = %q, want \"\" (scalar)", got)
	}

	// nil safety.
	if got := reg.ResponseWrapForSource(nil, "country"); got != "" {
		t.Errorf("ResponseWrapForSource(nil, country) = %q, want \"\"", got)
	}
}

// TestApplyGRPCPersist_AutoDerivesOnLROResponseShape is the end-to-end
// regression test for the user's reported bug: a sparse case (no wrap, no
// source) on a multi-message response must auto-derive both source and
// wrap via Shape 3. Pre-fix the request envelope leaked into the stub and
// EncodeResponse failed downstream.
func TestApplyGRPCPersist_AutoDerivesOnLROResponseShape(t *testing.T) {
	dir := t.TempDir()
	stubFile := filepath.Join(dir, "stubs", "countries", "MA.json")
	if err := os.MkdirAll(filepath.Dir(stubFile), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	initial, _ := json.Marshal(map[string]interface{}{
		"code":       "MA",
		"name":       "Morocco",
		"continent":  "Africa",
		"population": float64(37000000),
	})
	if err := os.WriteFile(stubFile, initial, 0644); err != nil {
		t.Fatalf("write stub: %v", err)
	}

	reg := loadLROTestRegistry(t)
	md, _ := reg.FindMethod("/lro.v1.CountryService/UpdateCountry")
	if md == nil {
		t.Fatal("UpdateCountry not found in LRO test registry")
	}

	h := &handler{
		loader:      &stubLoader{cfg: &config.Config{}, configDir: dir},
		transitions: newGRPCTransitionState(),
		registry:    reg,
	}

	// Sparse case — mirrors the user's `ready` transition case: no wrap,
	// no source. Must auto-derive both via Shape 3.
	c := config.Case{
		Status:  0,
		File:    "stubs/countries/{body.id}.json",
		Persist: true,
		Merge:   "update",
	}

	reqMap := map[string]interface{}{
		"regionId":  "EMEA",
		"charterId": "charter-1",
		"locale":    "fr-MA",
		"id":        "MA",
		"country": map[string]interface{}{
			"population": float64(37500000),
		},
	}

	code, handled, result, persistedPath := h.applyGRPCPersist(
		c, reqMap, time.Now(),
		"/lro.v1.CountryService/UpdateCountry", md,
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

	// Response must be wrapped under the auto-derived "country" envelope.
	inner, ok := result["country"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected result wrapped as {country: ...}, got %v", result)
	}
	if inner["population"] != float64(37500000) {
		t.Errorf("population not updated in wrapped response: %v", inner["population"])
	}

	stub := readStub(t, stubFile)

	// BUG REGRESSION: routing scalars and the nested wrapper key must NOT
	// appear in the persisted file.
	for _, key := range []string{"regionId", "charterId", "locale", "country"} {
		if _, has := stub[key]; has {
			t.Errorf("BUG REGRESSION: stub contains leaked key %q: %v", key, stub)
		}
	}
	// Population updated, other fields preserved.
	if stub["population"] != float64(37500000) {
		t.Errorf("population not merged: got %v", stub["population"])
	}
	if stub["name"] != "Morocco" || stub["continent"] != "Africa" {
		t.Errorf("seed fields lost: %v", stub)
	}
}

// TestApplyGRPCPersist_RefusesPayloadWithExtraEntityFields pins the
// runtime safeguard: when the persist payload (after source extraction
// and wrap filtering) still contains keys that aren't valid for the
// entity schema, the persist must be refused with codes.Internal and the
// stub file on disk must not be touched. This is the last-mile defence
// against config that slips past inheritance + auto-derive + the startup
// validator.
func TestApplyGRPCPersist_RefusesPayloadWithExtraEntityFields(t *testing.T) {
	dir := t.TempDir()
	stubFile := filepath.Join(dir, "stubs", "instances", "db-1.json")
	if err := os.MkdirAll(filepath.Dir(stubFile), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	initial, _ := json.Marshal(map[string]interface{}{
		"id":   "db-1",
		"tier": "standard",
	})
	if err := os.WriteFile(stubFile, initial, 0644); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	preCorrupt, _ := os.ReadFile(stubFile)

	reg := loadDatabaseTestRegistry(t)
	md, _ := reg.FindMethod("/database.v1.DatabaseService/UpdateDatabaseInstance")

	h := &handler{
		loader:      &stubLoader{cfg: &config.Config{}, configDir: dir},
		transitions: newGRPCTransitionState(),
		registry:    reg,
	}

	// Use an explicit source pointing at a request field WITH a non-
	// DatabaseInstance shape — this lets us inject a payload whose keys
	// don't match the entity schema, simulating a misconfigured route
	// that bypassed the validator. We point source at "name" (a string) —
	// ExtractSourceField will return nil → srcMap stays as reqMap (full
	// envelope), and with no wrap, the filter step is skipped. The
	// safeguard must catch the schema mismatch before persist.Update.
	c := config.Case{
		File:    "stubs/instances/{body.databaseInstance.id}.json",
		Persist: true,
		Merge:   "update",
		// No Source, no Wrap — sparse case. Auto-derive on
		// UpdateDatabaseInstance returns "databaseInstance" + "" (Shape 1
		// direct-entity), so wrap stays empty. We then send a payload
		// whose extracted DatabaseInstance has a key that's NOT a field
		// of DatabaseInstance — `bogusKey`.
	}
	reqMap := map[string]interface{}{
		"name": "db-1",
		"databaseInstance": map[string]interface{}{
			"id":       "db-1",
			"bogusKey": "would corrupt the file",
		},
	}

	code, handled, _, _ := h.applyGRPCPersist(c, reqMap, time.Now(),
		"/database.v1.DatabaseService/UpdateDatabaseInstance", md)
	if !handled {
		t.Fatal("expected handled=true on safeguard rejection")
	}
	if code != codes.Internal {
		t.Errorf("expected codes.Internal, got %v", code)
	}
	postCorrupt, _ := os.ReadFile(stubFile)
	if string(preCorrupt) != string(postCorrupt) {
		t.Errorf("stub file modified despite safeguard rejection:\n  pre:  %s\n  post: %s",
			preCorrupt, postCorrupt)
	}
}

// TestResponseWrap exercises the standalone wrap-from-response inference
// used by the explicit-source-implicit-wrap path in applyGRPCPersist.
func TestResponseWrap(t *testing.T) {
	reg := loadGeoTestRegistry(t)

	// UpdateCity: response-wrapped (UpdateCityResponse { City city = 1; }).
	// ResponseWrap must return "city" (the JSON name of the sole message field).
	md, _ := reg.FindMethod("/geo.v1.CityService/UpdateCity")
	if md == nil {
		t.Fatal("UpdateCity not found")
	}
	if got := reg.ResponseWrap(md); got != "city" {
		t.Errorf("ResponseWrap(UpdateCity) = %q, want %q", got, "city")
	}

	// PatchCity: direct-entity response (returns City directly). City has
	// only scalar fields, so ResponseWrap must return "" — there is no
	// wrapper; the response IS the entity.
	md, _ = reg.FindMethod("/geo.v1.CityService/PatchCity")
	if md == nil {
		t.Fatal("PatchCity not found")
	}
	if got := reg.ResponseWrap(md); got != "" {
		t.Errorf("ResponseWrap(PatchCity) = %q, want \"\" (direct-entity response has no wrap)", got)
	}

	// nil safety.
	if got := reg.ResponseWrap(nil); got != "" {
		t.Errorf("ResponseWrap(nil) = %q, want \"\"", got)
	}
}
