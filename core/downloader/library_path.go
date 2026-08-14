package downloader

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/model"
)

// downloadsSubfolder is where tool/Tidal output is filed within the target library, keeping
// downloader-sourced files visibly separate from the rest of the collection.
const downloadsSubfolder = "Downloads"

// invalidFilenameChars covers characters that are either path separators or invalid on
// common filesystems (Windows in particular); sanitizeFilename strips them.
var invalidFilenameChars = regexp.MustCompile(`[\\/:*?"<>|\x00-\x1f]`)

// sanitizeFilename reduces name to a safe base filename: no path separators, no traversal,
// no filesystem-hostile characters. Untrusted input (tool output filenames, Tidal
// Content-Disposition filenames) must always pass through this before touching the filesystem.
func sanitizeFilename(name string) string {
	name = filepath.Base(name)
	name = invalidFilenameChars.ReplaceAllString(name, "_")
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		return "download"
	}
	return name
}

// resolveTargetDir returns the absolute directory downloaded files for this job should be
// written into: <library path>/Downloads/<tool>.
func resolveTargetDir(ctx context.Context, ds model.DataStore, d *model.Download) (string, error) {
	libPath, err := ds.Library(ctx).GetPath(d.LibraryID)
	if err != nil {
		return "", fmt.Errorf("resolving library %d path: %w", d.LibraryID, err)
	}
	if libPath == "" {
		return "", fmt.Errorf("library %d has no configured path", d.LibraryID)
	}
	return filepath.Join(libPath, downloadsSubfolder, string(d.Tool)), nil
}

// stagingDirFor returns a per-job scratch directory, outside any library, that a tool downloads
// into. Writing here first (rather than straight into the library) means an interrupted or
// failed download never leaves a partial file where a concurrent scan could pick it up.
func stagingDirFor(d *model.Download) string {
	return filepath.Join(conf.Server.Downloader.StagingFolder.String(), d.ID)
}

// moveJobOutput moves every regular file produced under stagingDir into targetDir (flattening
// any subdirectories the tool created), sanitizing each filename, then removes stagingDir. It
// returns the number of files moved and, when at least one moved, the path of the last one
// (used only as a representative target_path; a job may produce several files, e.g. an album).
func moveJobOutput(stagingDir, targetDir string) (moved int, lastPath string, err error) {
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return 0, "", fmt.Errorf("creating target dir: %w", err)
	}
	walkErr := filepath.WalkDir(stagingDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		dest := filepath.Join(targetDir, sanitizeFilename(uniqueName(targetDir, d.Name())))
		if err := moveFile(path, dest); err != nil {
			return fmt.Errorf("moving %q: %w", d.Name(), err)
		}
		moved++
		lastPath = dest
		return nil
	})
	_ = os.RemoveAll(stagingDir)
	if walkErr != nil {
		return moved, lastPath, walkErr
	}
	return moved, lastPath, nil
}

// uniqueName appends a numeric suffix if name already exists in dir, so moving several
// same-named files (e.g. multiple "cover.jpg" from an album) never silently drops one.
func uniqueName(dir, name string) string {
	candidate := name
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 1; ; i++ {
		if _, err := os.Stat(filepath.Join(dir, candidate)); os.IsNotExist(err) {
			return candidate
		}
		candidate = fmt.Sprintf("%s (%d)%s", base, i, ext)
	}
}

func moveFile(src, dest string) error {
	if err := os.Rename(src, dest); err == nil {
		return nil
	}
	// Rename fails across filesystems/devices; fall back to copy+remove.
	in, err := os.Open(src) // #nosec -- src is produced by our own staging walk, not user input
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644) // #nosec
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Remove(src)
}
