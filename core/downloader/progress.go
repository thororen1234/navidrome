package downloader

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/navidrome/navidrome/model"
)

// ytDlpProgress matches yt-dlp's default stdout progress line, e.g.:
// "[download]  42.9% of    3.45MiB at    1.20MiB/s ETA 00:02"
var ytDlpProgress = regexp.MustCompile(`\[download]\s+([\d.]+)%`)

// spotdlProgress matches spotdl's default stdout progress line, e.g.:
// `Downloading "Song Name": 42%`
var spotdlProgress = regexp.MustCompile(`:\s*(\d+)%`)

// parseProgress extracts a 0.0-1.0 progress fraction and a status message from one line of a
// tool's stdout. ok is false when the line carries no progress signal - callers should still
// surface the raw line as a status message in that case if they want one.
//
// scdl and bandcamp-dl have no reliable machine-parseable progress in their default output, so
// they always return ok == false; jobs for those tools only ever report coarse queue state.
func parseProgress(tool model.DownloadTool, line string) (pct float64, msg string, ok bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return 0, "", false
	}
	switch tool {
	case model.DownloadToolYtDlp:
		if m := ytDlpProgress.FindStringSubmatch(line); m != nil {
			if v, err := strconv.ParseFloat(m[1], 64); err == nil {
				return v / 100, line, true
			}
		}
	case model.DownloadToolSpotdl:
		if m := spotdlProgress.FindStringSubmatch(line); m != nil {
			if v, err := strconv.Atoi(m[1]); err == nil {
				return float64(v) / 100, line, true
			}
		}
	}
	return 0, "", false
}
