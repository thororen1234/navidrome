package downloader

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/navidrome/navidrome/core/ffmpeg"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
)

// TidalDownloader is the subset of the Tidal client the worker needs to fetch a track or album
// into a local file. Defined here (rather than imported from core/tidal) so this package has no
// dependency on the Tidal integration; core/tidal.Client satisfies this interface. DownloadTo
// writes the raw response bytes to w: a single audio file for a track, or a zip archive for an
// album (TidalSubsonic always zips multi-track downloads on the fly). onSize receives the
// response's Content-Length (-1 if unknown) before any bytes are written.
type TidalDownloader interface {
	DownloadTo(ctx context.Context, tidalID, kind string, w io.Writer, onSize func(total int64)) (filename string, err error)
}

// maxZipEntrySize bounds a single extracted file, as a defensive cap against a malicious or
// corrupt zip response expanding without limit.
const maxZipEntrySize = 2 << 30 // 2GiB

// Share of a Tidal job's progress bar spent on the transfer itself; the rest covers extracting
// and remuxing, which have no finer-grained progress of their own.
const tidalTransferShare = 0.9

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
	// Runs before the staging RemoveAll (defers are LIFO) even if DownloadTo panics: an open
	// file can't be deleted on Windows, and would leak the handle everywhere.
	defer func() { _ = raw.Close() }()
	pw := &progressWriter{w: raw, total: -1, report: onProgress}
	filename, dlErr := w.tidal.DownloadTo(ctx, d.TidalID, d.TidalKind, pw, func(total int64) { pw.total = total })
	closeErr := raw.Close()
	if dlErr != nil {
		return 0, "", dlErr
	}
	if closeErr != nil {
		return 0, "", closeErr
	}
	onProgress(tidalTransferShare, "saving")

	files := filepath.Join(staging, "files")
	if err := os.MkdirAll(files, 0755); err != nil {
		return 0, "", err
	}
	if d.TidalKind == "album" {
		if err := extractZip(rawPath, files); err != nil {
			return 0, "", fmt.Errorf("extracting album zip: %w", err)
		}
		_ = os.Remove(rawPath)
		// Keep each album in its own folder (named after the zip, e.g. "Artist - Album"), so
		// albums don't all pile into one directory and fight over cover.jpg.
		if name := sanitizeFilename(strings.TrimSuffix(filename, filepath.Ext(filename))); name != "download" {
			targetDir = filepath.Join(targetDir, name)
		}
	} else {
		if err := os.Rename(rawPath, filepath.Join(files, sanitizeFilename(filename))); err != nil {
			return 0, "", err
		}
	}
	if err := remuxFragmented(ctx, files); err != nil {
		return 0, "", err
	}
	return moveJobOutput(files, targetDir)
}

// remuxFragmented rewrites any fragmented-MP4 audio file in dir (TidalSubsonic emits Tidal's DASH
// segments back to back for lossless tracks) into a regular .m4a, so the scanner reads its real
// duration rather than just the first ~4s segment's. Without ffmpeg the file is kept as-is.
func remuxFragmented(ctx context.Context, dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if frag, _ := ffmpeg.IsFragmentedMP4(path); !frag {
			continue
		}
		out := filepath.Join(dir, strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))+".m4a")
		tmp := out + ".remux"
		if err := ffmpeg.RemuxToMP4(ctx, path, tmp); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Warn(ctx, "Downloader: could not remux fragmented Tidal file; keeping it as-is", "file", e.Name(), err)
			_ = os.Remove(tmp)
			continue
		}
		if err := os.Remove(path); err != nil {
			return err
		}
		if err := os.Rename(tmp, out); err != nil {
			return err
		}
	}
	return nil
}

// progressWriter reports transfer progress as a fraction of total (scaled into the transfer
// share of the job's progress bar). With an unknown total it only reports bytes received.
type progressWriter struct {
	w       io.Writer
	total   int64
	written int64
	report  func(pct float64, msg string)
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n, err := p.w.Write(b)
	p.written += int64(n)
	if p.total > 0 {
		frac := min(float64(p.written)/float64(p.total), 1)
		p.report(frac*tidalTransferShare, fmt.Sprintf("downloading from Tidal: %s of %s",
			humanize.IBytes(uint64(p.written)), humanize.IBytes(uint64(p.total)))) //nolint:gosec // sizes are non-negative
	} else {
		p.report(0, "downloading from Tidal: "+humanize.IBytes(uint64(p.written))) //nolint:gosec
	}
	return n, err
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
