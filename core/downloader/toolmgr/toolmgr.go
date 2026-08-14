package toolmgr

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/downloader"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
)

// managedTools is every tool this package can install/upgrade/repair. Tidal is handled by
// core/tidal, not pip, so it is deliberately excluded.
var managedTools = []model.DownloadTool{
	model.DownloadToolYtDlp,
	model.DownloadToolScdl,
	model.DownloadToolSpotdl,
	model.DownloadToolBandcampDl,
	model.DownloadToolKhinsider,
}

var pipPackages = map[model.DownloadTool]string{
	model.DownloadToolYtDlp:      "yt-dlp",
	model.DownloadToolScdl:       "scdl",
	model.DownloadToolSpotdl:     "spotdl",
	model.DownloadToolBandcampDl: "bandcamp-downloader",
	model.DownloadToolKhinsider:  "khinsider-dl",
}

type Action string

const (
	ActionInstall Action = "install"
	ActionUpgrade Action = "upgrade"
	ActionRepair  Action = "repair"
)

type ToolStatus struct {
	Tool      model.DownloadTool `json:"tool"`
	Installed bool               `json:"installed"`
	Version   string             `json:"version,omitempty"`
}

type Manager interface {
	Status(ctx context.Context) []ToolStatus
	Run(ctx context.Context, tool model.DownloadTool, action Action) error
}

type manager struct{}

func New() Manager {
	return &manager{}
}

func (m *manager) Status(ctx context.Context) []ToolStatus {
	statuses := make([]ToolStatus, 0, len(managedTools))
	for _, tool := range managedTools {
		statuses = append(statuses, statusOf(ctx, tool))
	}
	return statuses
}

func statusOf(ctx context.Context, tool model.DownloadTool) ToolStatus {
	bin, err := downloader.ToolBin(tool)
	if err != nil {
		return ToolStatus{Tool: tool, Installed: false}
	}
	return ToolStatus{Tool: tool, Installed: true, Version: probeVersion(ctx, bin)}
}

func probeVersion(ctx context.Context, bin string) string {
	out, err := exec.CommandContext(ctx, bin, "--version").Output() // #nosec -- bin is a resolved, PATH-looked-up executable
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (m *manager) Run(ctx context.Context, tool model.DownloadTool, action Action) error {
	pkg, ok := pipPackages[tool]
	if !ok {
		return fmt.Errorf("unknown downloader tool %q", tool)
	}
	usePipx := conf.Server.Downloader.UsePipx
	bin, err := resolveInstallerBin(usePipx)
	if err != nil {
		return err
	}
	args := argsForAction(action, pkg, usePipx)
	if args == nil {
		return fmt.Errorf("unknown action %q", action)
	}
	log.Info(ctx, "Downloader: running tool installer", "tool", tool, "action", action, "bin", bin)
	cmd := exec.CommandContext(ctx, bin, args...) // #nosec -- bin resolved via PATH lookup, args from a fixed table
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s failed: %s", action, tool, truncate(strings.TrimSpace(string(out)), 2000))
	}
	return nil
}

// argsForAction returns the fixed argv (excluding the binary itself) for a pip/pipx action.
// pkg always comes from pipPackages, never from caller input.
func argsForAction(action Action, pkg string, usePipx bool) []string {
	if usePipx {
		switch action {
		case ActionInstall:
			return []string{"install", pkg}
		case ActionUpgrade:
			return []string{"upgrade", pkg}
		case ActionRepair:
			return []string{"reinstall", pkg}
		}
		return nil
	}
	switch action {
	case ActionInstall, ActionUpgrade:
		return []string{"install", "-U", pkg}
	case ActionRepair:
		return []string{"install", "--force-reinstall", pkg}
	}
	return nil
}

func resolveInstallerBin(usePipx bool) (string, error) {
	if usePipx {
		return exec.LookPath("pipx")
	}
	if conf.Server.Downloader.PipPath != "" {
		return exec.LookPath(conf.Server.Downloader.PipPath)
	}
	if p, err := exec.LookPath("pip3"); err == nil {
		return p, nil
	}
	return exec.LookPath("pip")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
