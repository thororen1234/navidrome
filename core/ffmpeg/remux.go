package ffmpeg

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/navidrome/navidrome/log"
)

// IsFragmentedMP4 reports whether the file at path is a fragmented MP4 (top-level "moof" boxes),
// e.g. DASH segments concatenated into one stream. Browsers and TagLib read the duration of such
// a file from its first fragment only (a few seconds), and can't seek in it, so these need a
// remux into a regular MP4 before they're played or added to a library.
func IsFragmentedMP4(path string) (bool, error) {
	f, err := os.Open(path) // #nosec -- callers pass their own cache/staging files
	if err != nil {
		return false, err
	}
	defer f.Close()
	return isFragmentedMP4(f)
}

func isFragmentedMP4(r io.ReadSeeker) (bool, error) {
	var hdr [16]byte
	sawFtyp := false
	for {
		if _, err := io.ReadFull(r, hdr[:8]); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return false, nil
			}
			return false, err
		}
		size := int64(binary.BigEndian.Uint32(hdr[:4]))
		boxType := string(hdr[4:8])
		headerLen := int64(8)
		switch size {
		case 0: // box extends to EOF
			return boxType == "moof", nil
		case 1: // 64-bit size follows the type
			if _, err := io.ReadFull(r, hdr[8:16]); err != nil {
				return false, nil //nolint:nilerr // truncated header: not a usable fMP4
			}
			size = int64(binary.BigEndian.Uint64(hdr[8:16]))
			headerLen = 16
		}
		if !sawFtyp {
			// Anything that doesn't open with an ftyp/styp box isn't an MP4 at all (FLAC, MP3...).
			if boxType != "ftyp" && boxType != "styp" {
				return false, nil
			}
			sawFtyp = true
		}
		if boxType == "moof" || boxType == "mvex" {
			return true, nil
		}
		if boxType == "moov" {
			// mvex (the fragmentation marker) lives inside moov; scan its children for it.
			frag, err := moovHasMvex(r, size-headerLen)
			if err != nil || frag {
				return frag, err
			}
			continue
		}
		if size < headerLen {
			return false, nil
		}
		if _, err := r.Seek(size-headerLen, io.SeekCurrent); err != nil {
			return false, err
		}
	}
}

func moovHasMvex(r io.ReadSeeker, remaining int64) (bool, error) {
	var hdr [8]byte
	for remaining >= 8 {
		if _, err := io.ReadFull(r, hdr[:]); err != nil {
			return false, nil //nolint:nilerr // truncated moov: treat as not fragmented
		}
		size := int64(binary.BigEndian.Uint32(hdr[:4]))
		if string(hdr[4:8]) == "mvex" {
			return true, nil
		}
		if size < 8 || size > remaining {
			return false, nil
		}
		if _, err := r.Seek(size-8, io.SeekCurrent); err != nil {
			return false, err
		}
		remaining -= size
	}
	return false, nil
}

// RemuxToMP4 rewrites the audio stream of in into a regular (non-fragmented, faststart) MP4 at
// out, without re-encoding and keeping its metadata. "-strict -2" is needed by older ffmpeg
// builds to write FLAC-in-MP4, which is what Tidal serves for lossless.
func RemuxToMP4(ctx context.Context, in, out string) error {
	cmd, err := ffmpegCmd()
	if err != nil {
		return err
	}
	args := []string{"-v", "error", "-y", "-i", in, "-map", "0:a:0", "-map_metadata", "0",
		"-c", "copy", "-strict", "-2", "-movflags", "+faststart", "-f", "mp4", out}
	log.Trace(ctx, "Remuxing fragmented MP4", "in", in, "out", out)
	output, err := exec.CommandContext(ctx, cmd, args...).CombinedOutput() // #nosec -- fixed argv, paths are our own files
	if err != nil {
		msg := strings.TrimSpace(string(output))
		if len(msg) > 1024 {
			msg = msg[:1024]
		}
		return fmt.Errorf("remuxing %s: %w: %s", in, err, msg)
	}
	return nil
}
