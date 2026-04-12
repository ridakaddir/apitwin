package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ridakaddir/apitwin/internal/logger"
	apitwinruntime "github.com/ridakaddir/apitwin/internal/runtime"
	"github.com/spf13/cobra"
)

var resetConfigFile string

var resetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Delete the runtime state directory so the next run starts from seed",
	Long: `Reset removes .apitwin/state/ next to the config so subsequent runs start
from a pristine mirror of the seed stubs.

Use this when you want to clear mutations from a previous session without
restarting the server (on restart the state dir is wiped and re-populated
automatically). For ephemeral runs nothing is persisted so this is a no-op.`,
	RunE: runReset,
}

func init() {
	resetCmd.Flags().StringVarP(&resetConfigFile, "config", "c", "", "Config file or directory (default: apitwin.toml if present, else current directory)")
	rootCmd.AddCommand(resetCmd)
}

func runReset(cmd *cobra.Command, args []string) error {
	cfg := resetConfigFile
	if cfg == "" {
		cfg = defaultConfig()
	}

	// Resolve to the config directory so we know where .apitwin/state lives.
	configDir := cfg
	info, err := os.Stat(cfg)
	if err != nil {
		return fmt.Errorf("resolving config path %q: %w", cfg, err)
	}
	if !info.IsDir() {
		configDir = filepath.Dir(cfg)
	}

	runtimeDir := apitwinruntime.DefaultPath(configDir)

	// Safety check: refuse to operate on any path that isn't recognisable as
	// an apitwin runtime state dir. This guards against a misconfigured
	// --config pointing at a system directory.
	if !apitwinruntime.IsRuntimePath(runtimeDir) {
		return fmt.Errorf("refusing to reset %q: not an apitwin runtime state directory", runtimeDir)
	}

	if _, err := os.Stat(runtimeDir); os.IsNotExist(err) {
		logger.Info("runtime state already clean", "dir", runtimeDir)
		return nil
	}

	if err := apitwinruntime.Cleanup(runtimeDir); err != nil {
		return fmt.Errorf("removing runtime state dir: %w", err)
	}

	logger.Info("runtime state cleared", "dir", runtimeDir)
	return nil
}
