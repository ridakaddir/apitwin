package cmd

import (
	"fmt"
	"strings"

	"github.com/ridakaddir/apitwin/internal/generate"
	"github.com/spf13/cobra"
)

var (
	genSpec   string
	genOut    string
	genFormat string
	genSplit  bool

	// proto generator flags
	genProtos      []string
	genImportPaths []string
)

var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate a apitwin config from an OpenAPI spec or .proto files",
	Long: `Generate reads an OpenAPI 3 spec (file or URL) or .proto files and produces:
  - A apitwin config file (TOML/YAML/JSON) with one route per operation/method
  - Stub JSON files in stubs/ populated from spec examples or synthesised from schemas

OpenAPI examples:
  apitwin generate --spec openapi.yaml --out ./mocks
  apitwin generate --spec https://petstore3.swagger.io/api/v3/openapi.json --out ./mocks
  apitwin generate --spec openapi.yaml --format yaml --split=false

Proto examples:
  apitwin generate --proto service.proto --out ./mocks
  apitwin generate --proto users.proto --proto orders.proto --out ./mocks
  apitwin generate --proto service.proto --import-path ./vendor/protos --format yaml`,
	RunE: runGenerate,
}

func init() {
	// OpenAPI flags.
	generateCmd.Flags().StringVarP(&genSpec, "spec", "s", "", "OpenAPI spec file path or URL")
	generateCmd.Flags().StringVarP(&genOut, "out", "o", "mocks", "Output directory for config files and stubs")
	generateCmd.Flags().StringVarP(&genFormat, "format", "f", "toml", "Config format: toml, yaml, json")
	generateCmd.Flags().BoolVar(&genSplit, "split", true, "Split routes into one file per tag (OpenAPI only; use --split=false for a single file)")

	// Proto flags.
	generateCmd.Flags().StringArrayVar(&genProtos, "proto", nil, "Path to a .proto file; repeat for multiple files (enables proto mode)")
	generateCmd.Flags().StringArrayVar(&genImportPaths, "import-path", nil, "Extra directory to search for proto imports; repeat for multiple paths")

	rootCmd.AddCommand(generateCmd)
}

func runGenerate(cmd *cobra.Command, args []string) error {
	// Proto mode: --proto takes precedence over --spec.
	if len(genProtos) > 0 {
		return runGenerateProto()
	}

	// OpenAPI mode: --spec is required.
	if genSpec == "" {
		return fmt.Errorf("either --spec (OpenAPI) or --proto (protobuf) is required")
	}
	return runGenerateOpenAPI()
}

func runGenerateOpenAPI() error {
	opts := generate.Options{
		Spec:   genSpec,
		OutDir: genOut,
		Format: genFormat,
		Split:  genSplit,
	}

	fmt.Printf("Loading spec: %s\n", genSpec)

	result, err := generate.Run(opts)
	if err != nil {
		return fmt.Errorf("generate failed: %w", err)
	}

	fmt.Printf("\nSpec: %s %s\n", result.SpecTitle, result.SpecVersion)
	fmt.Printf("Routes generated: %d\n", result.Routes)
	fmt.Printf("Stub files: %d\n", len(result.StubFiles))
	fmt.Printf("\nConfig files written:\n")
	for _, f := range result.ConfigFiles {
		fmt.Printf("  %s\n", f)
	}
	fmt.Printf("\nStub files written to: %s/stubs/\n", genOut)
	fmt.Printf("\nRun: apitwin --config %s\n", genOut)

	return nil
}

func runGenerateProto() error {
	opts := generate.ProtoOptions{
		ProtoFiles:  genProtos,
		ImportPaths: genImportPaths,
		OutDir:      genOut,
		Format:      genFormat,
	}

	fmt.Printf("Loading proto files: %s\n", strings.Join(genProtos, ", "))

	result, err := generate.RunProto(opts)
	if err != nil {
		return fmt.Errorf("proto generate failed: %w", err)
	}

	fmt.Printf("\ngRPC methods found: %d\n", result.Methods)
	fmt.Printf("Stub files: %d\n", len(result.StubFiles))
	fmt.Printf("\nConfig files written:\n")
	for _, f := range result.ConfigFiles {
		fmt.Printf("  %s\n", f)
	}
	fmt.Printf("\nStub files written to: %s/stubs/\n", genOut)
	fmt.Printf("\nRun: apitwin --config %s --grpc-proto %s\n", genOut, genProtos[0])

	return nil
}
