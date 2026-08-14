package downloader

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/navidrome/navidrome/model"
)

// TidalDownloader is the subset of the Tidal client the worker needs to fetch a track or album
// into a local file. Defined here (rather than imported from core/tidal) so this package has no
// dependency on the Tidal integration; core/tidal.Client satisfies this interface. DownloadTo
// writes the raw response bytes to w: a single audio file for a track, or a zip archive for an
// album (TidalSubsonic always zips multi-track downloads on the fly).
type TidalDownloader interface {
	DownloadTo(ctx context.Context, tidalID, kind string, w io.Writer) (filename string, err error)
}

// maxZipEntrySize bounds a single extracted file, as a defensive cap against a malicious or
// corrupt zip response expanding without limit.
const maxZipEntrySize = 2 << 30 // 2GiB

func (w *Worker) runTidalJob(ctx context.Context, d *model.Download, targetDir string, onProgress func(pct float64, msg string)) (moved int, lastPath string, err error) {
	if w.tidal == nil {
		return 0, "", errors.New("tidal integration is not configured")
	}
	staging := stagingDirFor(d)
	defer func() { _ = os.RemoveAll(staging) }()
	if err := os.MkdirAll(staging, 0755); err != nil {
		return 0, "", fmt.Errorf("creating staging dir: %w", err)
	}

	onProgress(0, "downloading from Tidal")
	rawPath := filepath.Join(staging, "download.raw")
	raw, err := os.Create(rawPath) // #nosec -- path is our own staging dir, not user input
	if err != nil {
		return 0, "", err
	}
	filename, dlErr := w.tidal.DownloadTo(ctx, d.TidalID, d.TidalKind, raw)
	closeErr := raw.Close()
	if dlErr != nil {
		return 0, "", dlErr
	}
	if closeErr != nil {
		return 0, "", closeErr
	}
	onProgress(0.9, "saving")

	if d.TidalKind == "album" {
		if err := extractZip(rawPath, staging); err != nil {
			return 0, "", fmt.Errorf("extracting album zip: %w", err)
		}
		_ = os.Remove(rawPath)
	} else {
		dest := filepath.Join(staging, sanitizeFilename(filename))
		if dest != rawPath {
			if err := os.Rename(rawPath, dest); err != nil {
				return 0, "", err
			}
		}
	}
	return moveJobOutput(staging, targetDir)
}

// extractZip extracts every file entry of the zip at zipPath into destDir, flattening any
// directory structure and sanitizing names. Using filepath.Base (rather than the entry's
// original path) is deliberate zip-slip protection: a malicious "../../etc/passwd" entry name
// can never escape destDir.
func extractZip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := sanitizeFilename(filepath.Base(f.Name))
		dest := filepath.Join(destDir, uniqueName(destDir, name))
		if err := extractZipEntry(f, dest); err != nil {
			return fmt.Errorf("extracting %q: %w", f.Name, err)
		}
	}
	return nil
}

func extractZipEntry(f *zip.File, dest string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644) // #nosec -- dest built from sanitizeFilename
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, io.LimitReader(rc, maxZipEntrySize))
	return err
}
