package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func sampleHTTPRoute() Route {
	enabled := true
	return Route{
		Method:   "GET",
		Match:    "/continents/{id}",
		Enabled:  &enabled,
		Fallback: "success",
		Conditions: []Condition{
			{Source: "query", Field: "expand", Op: "eq", Value: "countries", Case: "expanded"},
		},
		Cases: map[string]Case{
			"success":  {Status: 200, JSON: `{"id":"af","name":"Africa"}`},
			"expanded": {Status: 200, File: "stubs/africa-expanded.json"},
		},
		Transitions: []Transition{
			{Case: "success", Duration: 5},
			{Case: "expanded"},
		},
	}
}

func sampleGRPCRoute() GRPCRoute {
	return GRPCRoute{
		Match:    "/atlas.v1.Continents/Get",
		Fallback: "ok",
		Cases: map[string]Case{
			"ok": {Status: 0, JSON: `{"name":"Africa"}`},
		},
	}
}

func TestMarshalRoundTrip(t *testing.T) {
	cfg := &Config{
		Routes:     []Route{sampleHTTPRoute()},
		GRPCRoutes: []GRPCRoute{sampleGRPCRoute()},
	}

	for _, format := range []string{"toml", "yaml", "json"} {
		t.Run(format, func(t *testing.T) {
			data, err := Marshal(cfg, format)
			if err != nil {
				t.Fatalf("marshal %s: %v", format, err)
			}
			if len(data) == 0 {
				t.Fatalf("marshal %s returned empty bytes", format)
			}

			dir := t.TempDir()
			path := filepath.Join(dir, "atlas."+format)
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatalf("write: %v", err)
			}
			loaded, err := Load(path)
			if err != nil {
				t.Fatalf("load %s: %v\n--- bytes ---\n%s", format, err, string(data))
			}
			if len(loaded.Routes) != 1 || loaded.Routes[0].Match != "/continents/{id}" {
				t.Fatalf("routes round-trip lost data: %+v", loaded.Routes)
			}
			if len(loaded.GRPCRoutes) != 1 || loaded.GRPCRoutes[0].Match != "/atlas.v1.Continents/Get" {
				t.Fatalf("grpc routes round-trip lost data: %+v", loaded.GRPCRoutes)
			}
			if got := loaded.Routes[0].Cases["expanded"].File; got != "stubs/africa-expanded.json" {
				t.Fatalf("case file lost: %q", got)
			}
			if !loaded.Routes[0].IsEnabled() {
				t.Fatalf("enabled lost: %+v", loaded.Routes[0].Enabled)
			}
		})
	}
}

func TestUpsertRouteInsertAndReplace(t *testing.T) {
	for _, format := range []string{"toml", "yaml", "json"} {
		t.Run(format, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "atlas."+format)

			if err := UpsertRouteInFile(path, KindHTTP, sampleHTTPRoute(), nil); err != nil {
				t.Fatalf("insert into new file: %v", err)
			}
			cfg, err := Load(path)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			if len(cfg.Routes) != 1 {
				t.Fatalf("expected 1 route, got %d", len(cfg.Routes))
			}

			r2 := sampleHTTPRoute()
			r2.Match = "/countries/{id}"
			if err := UpsertRouteInFile(path, KindHTTP, r2, nil); err != nil {
				t.Fatalf("insert second route: %v", err)
			}
			cfg, _ = Load(path)
			if len(cfg.Routes) != 2 {
				t.Fatalf("expected 2 routes, got %d", len(cfg.Routes))
			}

			updated := sampleHTTPRoute()
			updated.Fallback = "expanded"
			err = UpsertRouteInFile(path, KindHTTP, updated, &Selector{
				Kind: KindHTTP, Match: "/continents/{id}", Method: "GET",
			})
			if err != nil {
				t.Fatalf("replace: %v", err)
			}
			cfg, _ = Load(path)
			if cfg.Routes[0].Fallback != "expanded" {
				t.Fatalf("replace did not stick: fallback=%q", cfg.Routes[0].Fallback)
			}
			if len(cfg.Routes) != 2 {
				t.Fatalf("replace changed route count: %d", len(cfg.Routes))
			}

			err = UpsertRouteInFile(path, KindHTTP, sampleHTTPRoute(), &Selector{
				Kind: KindHTTP, Match: "/does-not-exist", Method: "GET",
			})
			if !errors.Is(err, ErrRouteNotFound) {
				t.Fatalf("expected ErrRouteNotFound, got %v", err)
			}

			err = UpsertRouteInFile(path, KindHTTP, sampleHTTPRoute(), nil)
			if !errors.Is(err, ErrRouteConflict) {
				t.Fatalf("expected ErrRouteConflict, got %v", err)
			}
		})
	}
}

func TestDeleteRouteFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "atlas.toml")
	if err := UpsertRouteInFile(path, KindHTTP, sampleHTTPRoute(), nil); err != nil {
		t.Fatalf("seed: %v", err)
	}
	err := DeleteRouteFromFile(path, Selector{
		Kind: KindHTTP, Match: "/continents/{id}", Method: "GET",
	})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	cfg, _ := Load(path)
	if len(cfg.Routes) != 0 {
		t.Fatalf("expected 0 routes after delete, got %d", len(cfg.Routes))
	}

	if err := DeleteRouteFromFile(path, Selector{
		Kind: KindHTTP, Match: "/continents/{id}", Method: "GET",
	}); !errors.Is(err, ErrRouteNotFound) {
		t.Fatalf("expected ErrRouteNotFound, got %v", err)
	}
}

func TestFindFileContainingRoute(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "africa.toml")
	b := filepath.Join(dir, "europe.yaml")
	if err := UpsertRouteInFile(a, KindHTTP, sampleHTTPRoute(), nil); err != nil {
		t.Fatalf("seed a: %v", err)
	}
	r2 := sampleHTTPRoute()
	r2.Match = "/cities/{id}"
	if err := UpsertRouteInFile(b, KindHTTP, r2, nil); err != nil {
		t.Fatalf("seed b: %v", err)
	}

	got, err := FindFileContainingRoute(dir, Selector{
		Kind: KindHTTP, Match: "/cities/{id}", Method: "GET",
	})
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got != b {
		t.Fatalf("expected %s, got %s", b, got)
	}

	got, err = FindFileContainingRoute(dir, Selector{
		Kind: KindHTTP, Match: "/nope", Method: "GET",
	})
	if err != nil {
		t.Fatalf("find missing: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty for missing route, got %s", got)
	}
}

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "atlas.toml")

	if err := WriteFileAtomic(path, []byte("# africa\n")); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if err := WriteFileAtomic(path, []byte("# europe\n")); err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "# europe\n" {
		t.Fatalf("expected europe, got %q", string(got))
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, e := range entries {
		if e.Name() != "atlas.toml" {
			t.Fatalf("leftover temp file: %s", e.Name())
		}
	}
}

func TestMarshalRoutePreview(t *testing.T) {
	for _, format := range []string{"toml", "yaml", "json"} {
		t.Run(format, func(t *testing.T) {
			data, err := MarshalRoute(KindHTTP, sampleHTTPRoute(), format)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if len(data) == 0 {
				t.Fatalf("empty preview")
			}
		})
	}
}
