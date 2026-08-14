package downloader

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
)

// ToolBin resolves a tool's binary: the configured path if set, otherwise the bare command
// name looked up on PATH. Deliberately uncached (unlike ffmpeg's ffmpegCmd): the toolmgr
// package's Install/Upgrade/Repair actions must be reflected on the very next resolution, and
// it reuses this same function to report install status.
func ToolBin(tool model.DownloadTool) (string, error) {
	configured, fallback := toolPathConfig(tool)
	path := configured
	if path == "" {
		path = fallback
	}
	resolved, err := exec.LookPath(path)
	if err != nil {
		return "", fmt.Errorf("%s not found (looked for %q): %w", tool, path, err)
	}
	return resolved, nil
}

func toolPathConfig(tool model.DownloadTool) (configured, fallback string) {
	switch tool {
	case model.DownloadToolYtDlp:
		return conf.Server.Downloader.YtDlpPath, "yt-dlp"
	case model.DownloadToolScdl:
		return conf.Server.Downloader.ScdlPath, "scdl"
	case model.DownloadToolSpotdl:
		return conf.Server.Downloader.SpotdlPath, "spotdl"
	case model.DownloadToolBandcampDl:
		return conf.Server.Downloader.BandcampDlPath, "bandcamp-dl"
	}
	return "", ""
}

// buildArgs returns the fixed-shape argv for a tool invocation. sourceURL is always the last
// element and is never shell-interpolated; validate() already rejects anything not starting
// with http(s):// so it cannot be mistaken for a flag.
func buildArgs(tool model.DownloadTool, bin, sourceURL, outDir string) []string {
	switch tool {
	case model.DownloadToolYtDlp:
		outTemplate := filepath.Join(outDir, "%(title)s.%(ext)s")
		return []string{bin, "-x", "--audio-format", "mp3", "--audio-quality", "0",
			"--newline", "-o", outTemplate, sourceURL}
	case model.DownloadToolScdl:
		return []string{bin, "-l", sourceURL, "--path", outDir, "--onlymp3"}
	case model.DownloadToolSpotdl:
		outTemplate := filepath.Join(outDir, "{title}.{output-ext}")
		return []string{bin, "download", sourceURL, "--output", outTemplate}
	case model.DownloadToolBandcampDl:
		return []string{bin, "--base-dir", outDir, sourceURL}
	}
	return nil
}

// runExecJob shells out to an external downloader tool (yt-dlp/scdl/spotdl/bandcamp-dl),
// following core/ffmpeg's safe-exec discipline: argv is always a Go slice passed straight to
// exec.CommandContext, never a shell string. onProgress is called for each stdout line that
// parseProgress recognizes as a progress update.
func runExecJob(ctx context.Context, d *model.Download, outDir string, onProgress func(pct float64, msg string)) error {
	bin, err := ToolBin(d.Tool)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("creating staging dir: %w", err)
	}
	args := buildArgs(d.Tool, bin, d.SourceURL, outDir)
	if args == nil {
		return fmt.Errorf("no command template for tool %q", d.Tool)
	}

	log.Debug(ctx, "Downloader: executing job", "tool", d.Tool, "id", d.ID)
	cmd := exec.CommandContext(ctx, args[0], args[1:]...) // #nosec
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("starting %s: %w", d.Tool, err)
	}
	stderrBuf := &limitedBuffer{limit: 4096}
	cmd.Stderr = stderrBuf

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting %s: %w", d.Tool, err)
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 4096), 1<<16)
	var lastMsg string
	for scanner.Scan() {
		line := scanner.Text()
		if pct, msg, ok := parseProgress(d.Tool, line); ok {
			onProgress(pct, msg)
			lastMsg = msg
		} else if strings.TrimSpace(line) != "" {
			lastMsg = strings.TrimSpace(line)
		}
	}

	if err := cmd.Wait(); err != nil {
		detail := strings.TrimSpace(stderrBuf.String())
		if detail == "" {
			detail = lastMsg
		}
		if detail != "" {
			return fmt.Errorf("%s failed: %s", d.Tool, detail)
		}
		return fmt.Errorf("%s failed: %w", d.Tool, err)
	}
	return nil
}

// limitedBuffer caps how much stderr it retains, so a chatty subprocess can't grow this
// unbounded while the job runs.
type limitedBuffer struct {
	buf   bytes.Buffer
	limit int
}

func (w *limitedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	remaining := w.limit - w.buf.Len()
	if remaining <= 0 {
		return n, nil
	}
	if len(p) > remaining {
		p = p[:remaining]
	}
	w.buf.Write(p)
	return n, nil
}

func (w *limitedBuffer) String() string { return w.buf.String() }

var _ io.Writer = (*limitedBuffer)(nil)
