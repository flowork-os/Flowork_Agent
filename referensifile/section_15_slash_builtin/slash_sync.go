package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/teetah2402/flowork/internal/settingsync"
)

// handleSyncCommand exposes settingsync.Export/Import as /sync slash.
// Usage:
//
//	/sync export [path]         — write bundle to path (or ~/.flowork/sync/<timestamp>.tar.gz)
//	/sync import <path>         — restore bundle into ~/.flowork/ (non-overwrite)
//	/sync import --force <path> — overwrite existing files
func (m *model) handleSyncCommand(args []string) {
	if len(args) == 0 {
		m.appendLocal("SYNC", "Usage:\n  /sync export [path]\n  /sync import [--force] <path>")
		return
	}
	switch args[0] {
	case "export":
		dest := ""
		if len(args) > 1 {
			dest = args[1]
		}
		if dest == "" {
			home, _ := os.UserHomeDir()
			ts := time.Now().Format("20060102-150405")
			dest = filepath.Join(home, ".flowork", "sync", fmt.Sprintf("flowork-settings-%s.tar.gz", ts))
		}
		bundle, err := settingsync.Export(dest)
		if err != nil {
			m.appendLocal("SYNC", fmt.Sprintf("✗ export failed: %v", err))
			return
		}
		manifestPath := dest + ".manifest.json"
		_ = settingsync.WriteManifest(manifestPath, bundle)
		m.appendLocal("SYNC", fmt.Sprintf("✓ exported %s\n  %s\n  manifest: %s",
			dest, bundle.Summary(), manifestPath))
	case "import":
		force := false
		path := ""
		for _, a := range args[1:] {
			switch a {
			case "--force", "-f":
				force = true
			default:
				path = a
			}
		}
		if path == "" {
			m.appendLocal("SYNC", "Usage: /sync import [--force] <path>")
			return
		}
		written, err := settingsync.Import(path, force)
		if err != nil {
			m.appendLocal("SYNC", fmt.Sprintf("✗ import failed: %v", err))
			return
		}
		m.appendLocal("SYNC", fmt.Sprintf("✓ imported %d file(s) from %s", len(written), path))
	default:
		m.appendLocal("SYNC", fmt.Sprintf("Unknown subcommand %q. Use export or import.", args[0]))
	}
}
