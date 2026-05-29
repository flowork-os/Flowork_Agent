package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/teetah2402/flowork/internal/selfupdate"
)

// Version is stamped at build time or defaults to the repo version.
// Can be overridden via -ldflags "-X github.com/teetah2402/flowork/internal/tui.Version=0.3.1"
var Version = "0.3.1"

// GitHubOwner / GitHubRepo override env vars. Keep in one place so
// /update command knows where to look. Default matches module path.
var (
	GitHubOwner = envOr("FLOWORK_GH_OWNER", "teetah2402")
	GitHubRepo  = envOr("FLOWORK_GH_REPO", "flowork")
)

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// handleUpdateCommand checks GitHub for a newer release and optionally applies it.
// Usage:
//
//	/update              — check only, show result
//	/update --apply      — download & swap binary (restart needed after)
//	/update --channel beta
func (m *model) handleUpdateCommand(args []string) {
	apply := false
	channel := "stable"
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--apply", "apply", "-y":
			apply = true
		case "--channel":
			if i+1 < len(args) {
				channel = args[i+1]
				i++
			}
		default:
			// no-op — exhaustive switch guard
		}
	}

	cfg := selfupdate.DefaultConfig()
	cfg.Owner = GitHubOwner
	cfg.Repo = GitHubRepo
	cfg.Channel = channel
	cfg.CurrentVer = Version

	u := selfupdate.New(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	m.appendLocal("UPDATE", fmt.Sprintf("🔍 Checking GitHub releases (%s/%s, channel=%s)...", cfg.Owner, cfg.Repo, cfg.Channel))
	rel, src, err := u.CheckOnly(ctx)
	if err != nil {
		m.appendLocal("UPDATE", fmt.Sprintf("✗ Check failed: %v\n(mesh fallback not yet wired — will be added next sprint)", err))
		return
	}
	if rel == nil {
		m.appendLocal("UPDATE", "No release found in the selected channel. Check owner/repo config.")
		return
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Source:       %s\n", src)
	fmt.Fprintf(&sb, "Latest:       %s (%s)\n", rel.Version, rel.Tag)
	fmt.Fprintf(&sb, "Current:      %s\n", cfg.CurrentVer)
	fmt.Fprintf(&sb, "Published:    %s\n", rel.PublishedAt.Format("2006-01-02 15:04"))
	fmt.Fprintf(&sb, "Assets:       %d\n", len(rel.Assets))

	if !u.IsNewer(rel) {
		sb.WriteString("\n✓ Already up to date (or current is newer).\n")
		m.appendLocal("UPDATE", sb.String())
		return
	}

	asset := selfupdate.PickAsset(rel, false)
	if asset == nil {
		sb.WriteString("\n✗ No asset matches current OS/arch. Release may not include your platform.\n")
		m.appendLocal("UPDATE", sb.String())
		return
	}
	fmt.Fprintf(&sb, "Picked asset: %s (%s/%s, %d bytes)\n", asset.Name, asset.OS, asset.Arch, asset.Size)

	if !apply {
		sb.WriteString("\n⚠ This is a check only. Use '/update --apply' to download + install.\n")
		sb.WriteString("Safety: owner files (.env, config, sessions, memory, prompts) are never touched.\n")
		m.appendLocal("UPDATE", sb.String())
		return
	}

	sb.WriteString("\n⬇ Downloading and swapping binary...\n")
	m.appendLocal("UPDATE", sb.String())

	applyCtx, applyCancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer applyCancel()
	backup, err := u.ApplyFrom(applyCtx, rel, src)
	if err != nil {
		m.appendLocal("UPDATE", fmt.Sprintf("✗ Apply failed: %v", err))
		return
	}
	m.appendLocal("UPDATE", fmt.Sprintf("✓ Installed %s (source: %s). Backup: %s\nRestart flowork to run the new version. Use /rollback if something breaks.", rel.Version, src, backup))
}

// handleRollbackCommand restores the most recent backup.
func (m *model) handleRollbackCommand() {
	backups, err := selfupdate.ListBackups()
	if err != nil {
		m.appendLocal("ROLLBACK", fmt.Sprintf("✗ %v", err))
		return
	}
	if len(backups) == 0 {
		m.appendLocal("ROLLBACK", "No backup available. /update hasn't run yet.")
		return
	}
	restored, err := selfupdate.Rollback()
	if err != nil {
		m.appendLocal("ROLLBACK", fmt.Sprintf("✗ %v", err))
		return
	}
	m.appendLocal("ROLLBACK", fmt.Sprintf("✓ Restored from %s\nRestart flowork to run the previous version.", restored))
}
