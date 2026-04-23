package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/ridakaddir/apitwin/internal/config"
	apitwingrpc "github.com/ridakaddir/apitwin/internal/grpc"
	"github.com/ridakaddir/apitwin/internal/logger"
	"github.com/ridakaddir/apitwin/internal/proxy"
	"github.com/spf13/cobra"
)

// defaultConfig resolves the config path to use when --config is not set:
//   - "./apitwin.toml" if it exists (single-file default)
//   - "."              otherwise (load all config files in current directory)
func defaultConfig() string {
	if _, err := os.Stat("apitwin.toml"); err == nil {
		return "apitwin.toml"
	}
	return "."
}

// version is set at build time via -ldflags "-X github.com/ridakaddir/apitwin/cmd.version=<tag>"
// Falls back to "dev" for local builds without a tag.
var version = "dev"

var (
	target       string
	port         int
	configFile   string
	apiPrefix    string
	recordMode   bool
	noMaskPII    bool
	initMode     bool
	noRuntimeDir bool
	ephemeral    bool
	resetRuntime bool

	// gRPC flags — server only starts when --grpc-proto is provided.
	grpcPort        int
	grpcProtos      []string
	grpcTarget      string
	grpcImportPaths []string
)

var rootCmd = &cobra.Command{
	Use:     "apitwin",
	Short:   "apitwin — mock, stub and proxy APIs for frontend development",
	Version: version,
	RunE:    run,
}

// Execute is the entrypoint called from main.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().StringVarP(&target, "target", "t", "", "Upstream API URL to proxy unmatched requests to")
	rootCmd.Flags().IntVarP(&port, "port", "p", 4000, "Port to listen on")
	rootCmd.Flags().StringVarP(&configFile, "config", "c", "", "Config file or directory (default: apitwin.toml if present, else current directory)")
	rootCmd.Flags().StringVarP(&apiPrefix, "api-prefix", "a", "", "Strip this prefix from request paths before matching routes and forwarding to upstream (e.g. /api)")
	rootCmd.Flags().BoolVar(&recordMode, "record", false, "Record mode: proxy all requests and save responses as stubs (PII is masked by default — pass --no-mask-pii to disable)")
	rootCmd.Flags().BoolVar(&noMaskPII, "no-mask-pii", false, "Disable PII masking on recorded stubs (only meaningful with --record)")
	rootCmd.Flags().BoolVar(&initMode, "init", false, "Scaffold an apitwin.toml template in the current directory")
	rootCmd.Flags().BoolVar(&ephemeral, "ephemeral", false, "Mirror stubs into a tempdir wiped on shutdown; nothing lands on disk next to the config")
	rootCmd.Flags().BoolVar(&noRuntimeDir, "no-runtime-dir", false, "Write mutations back to seed stubs (legacy, dirties git)")
	rootCmd.Flags().BoolVar(&resetRuntime, "reset-runtime", false, "Wipe the runtime state directory on start and re-mirror from seed (default preserves runtime-only stubs and persisted transition state across restarts)")

	// gRPC flags.
	rootCmd.Flags().IntVar(&grpcPort, "grpc-port", 50051, "gRPC server port (only used when --grpc-proto is provided)")
	rootCmd.Flags().StringArrayVar(&grpcProtos, "grpc-proto", nil, "Path to a .proto file; repeat for multiple files (enables the gRPC server)")
	rootCmd.Flags().StringVar(&grpcTarget, "grpc-target", "", "Upstream gRPC server for proxy/forward mode (e.g. localhost:9090)")
	rootCmd.Flags().StringArrayVar(&grpcImportPaths, "import-path", nil, "Extra directory to search for proto imports; repeat for multiple paths")
}

func run(cmd *cobra.Command, args []string) error {
	// --init: scaffold config files and exit.
	if initMode {
		if err := proxy.Init("."); err != nil {
			return fmt.Errorf("init failed: %w", err)
		}
		logger.Info("scaffolded apitwin.toml and stubs/users.json")
		logger.Info("run: apitwin --target https://api.example.com")
		return nil
	}

	// Resolve config path: explicit flag > apitwin.toml > current directory.
	cfg := configFile
	if cfg == "" {
		cfg = defaultConfig()
	}

	// Require --target if record mode is on (nothing to proxy otherwise).
	if recordMode && target == "" {
		return fmt.Errorf("--record requires --target")
	}

	opts := proxy.ServerOptions{
		Target:       target,
		Port:         port,
		ConfigPath:   cfg,
		ApiPrefix:    apiPrefix,
		RecordMode:   recordMode,
		MaskPII:      !noMaskPII,
		NoRuntimeDir: noRuntimeDir,
		Ephemeral:    ephemeral,
		ResetRuntime: resetRuntime,
	}

	srv, err := proxy.NewServer(opts)
	if err != nil {
		return fmt.Errorf("starting server: %w", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Start the gRPC server only when --grpc-proto is provided.
	if len(grpcProtos) > 0 {
		grpcSrv, err := apitwingrpc.NewServer(apitwingrpc.ServerOptions{
			ProtoFiles:  grpcProtos,
			ImportPaths: grpcImportPaths,
			Target:      grpcTarget,
			Port:        grpcPort,
			Loader:      srv.Loader(),
			MetaDir:     srv.MetaDir(),
		})
		if err != nil {
			return fmt.Errorf("starting gRPC server: %w", err)
		}
		// Wire config hot-reload so gRPC transitions reset alongside HTTP ones.
		srv.Loader().AddOnChange(func(_ *config.Config) {
			grpcSrv.NotifyReload()
		})
		// Wire the devtool UI so the browser can dispatch JSON-shaped gRPC
		// calls through /__api/grpc/invoke.
		srv.SetGRPCInvoker(grpcSrv.Invoker())
		go func() {
			if err := grpcSrv.Start(ctx); err != nil {
				logger.Error("gRPC server error", "err", err)
			}
		}()
	}

	return srv.Start(ctx)
}
