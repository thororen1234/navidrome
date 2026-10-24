package tidal

import (
	"context"
	"crypto/md5" //nolint:gosec // only used to derive a filesystem-safe cache key
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/navidrome/navidrome/core/ffmpeg"
	"github.com/navidrome/navidrome/log"
	"golang.org/x/sync/singleflight"
)

// CachedTrack is a fully-downloaded, seekable local copy of a Tidal track, ready to be served
// with http.ServeContent.
type CachedTrack struct {
	Path        string
	ContentType string
}

// streamCache materializes each streamed track into a local file before it is played.
// TidalSubsonic's stream endpoint can't be proxied straight through to the browser: for
// lossless/hi-res tracks it emits Tidal's DASH segments back to back (a fragmented MP4), which
// browsers report as only ~4s long (the first segment) and can't seek in. Downloading once and
// remuxing into a regular MP4 gives the player a real duration, Content-Length and Range support.
type streamCache struct {
	dir      string
	maxBytes int64
	fetch    func(ctx context.Context, id string, w io.Writer) (contentType string, err error)
	remux    func(ctx context.Context, in, out string) error
	timeout  time.Duration
	group    singleflight.Group
	pruneMu  sync.Mutex
}

// staleTempAge is how old a leftover ".part"/".remux" file must be before prune treats it as
// abandoned (e.g. from a crash mid-download) rather than an in-progress fill.
const staleTempAge = time.Hour

var safeKeyRE = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func cacheKey(id string) string {
	if safeKeyRE.MatchString(id) {
		return id
	}
	sum := md5.Sum([]byte(id)) //nolint:gosec
	return hex.EncodeToString(sum[:])
}

var audioExtByType = map[string]string{
	"audio/flac":   ".flac",
	"audio/x-flac": ".flac",
	"audio/mp4":    ".m4a",
	"audio/x-m4a":  ".m4a",
	"audio/m4a":    ".m4a",
	"video/mp4":    ".m4a",
	"audio/mpeg":   ".mp3",
	"audio/mp3":    ".mp3",
	"audio/aac":    ".aac",
	"audio/ogg":    ".ogg",
	"audio/opus":   ".opus",
	"audio/wav":    ".wav",
	"audio/x-wav":  ".wav",
}

var contentTypeByExt = map[string]string{
	".flac": "audio/flac",
	".m4a":  "audio/mp4",
	".mp3":  "audio/mpeg",
	".aac":  "audio/aac",
	".ogg":  "audio/ogg",
	".opus": "audio/ogg",
	".wav":  "audio/wav",
}

// extForContentType maps an upstream Content-Type to a file extension, sniffing the file's first
// bytes when the header is missing or generic (application/octet-stream).
func extForContentType(contentType, path string) string {
	mediaType, _, _ := mime.ParseMediaType(contentType)
	if ext, ok := audioExtByType[strings.ToLower(mediaType)]; ok {
		return ext
	}
	if f, err := os.Open(path); err == nil { // #nosec -- our own cache file
		defer f.Close()
		head := make([]byte, 512)
		n, _ := io.ReadFull(f, head)
		head = head[:n]
		switch {
		case strings.HasPrefix(string(head), "fLaC"):
			return ".flac"
		case n >= 8 && string(head[4:8]) == "ftyp":
			return ".m4a"
		}
		mediaType, _, _ = mime.ParseMediaType(http.DetectContentType(head))
		if ext, ok := audioExtByType[mediaType]; ok {
			return ext
		}
	}
	return ".m4a"
}

func (c *streamCache) lookup(key string) (CachedTrack, bool) {
	for ext, ct := range contentTypeByExt {
		p := filepath.Join(c.dir, key+ext)
		if _, err := os.Stat(p); err == nil {
			now := time.Now()
			_ = os.Chtimes(p, now, now) // LRU bookkeeping for prune
			return CachedTrack{Path: p, ContentType: ct}, true
		}
	}
	return CachedTrack{}, false
}

// Get returns the cached copy of track id, downloading (and remuxing if needed) on first use.
// Concurrent requests for the same track - e.g. the browser's initial request plus an immediate
// Range retry - share one download. The download runs detached from the request's context, so a
// client that gives up mid-fill doesn't throw away work the next request will want.
func (c *streamCache) Get(ctx context.Context, id string) (CachedTrack, error) {
	key := cacheKey(id)
	if t, ok := c.lookup(key); ok {
		return t, nil
	}
	ch := c.group.DoChan(key, func() (any, error) {
		if t, ok := c.lookup(key); ok {
			return t, nil
		}
		fillCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), c.timeout)
		defer cancel()
		return c.fill(fillCtx, key, id)
	})
	select {
	case <-ctx.Done():
		return CachedTrack{}, ctx.Err()
	case res := <-ch:
		if res.Err != nil {
			return CachedTrack{}, res.Err
		}
		return res.Val.(CachedTrack), nil
	}
}

func (c *streamCache) fill(ctx context.Context, key, id string) (CachedTrack, error) {
	if err := os.MkdirAll(c.dir, 0755); err != nil {
		return CachedTrack{}, fmt.Errorf("creating tidal stream cache dir: %w", err)
	}
	part := filepath.Join(c.dir, key+".part")
	f, err := os.Create(part) // #nosec -- key is sanitized by cacheKey
	if err != nil {
		return CachedTrack{}, err
	}
	start := time.Now()
	contentType, fetchErr := c.fetch(ctx, id, f)
	closeErr := f.Close()
	if fetchErr == nil {
		fetchErr = closeErr
	}
	if fetchErr != nil {
		_ = os.Remove(part)
		return CachedTrack{}, fetchErr
	}

	final := ""
	if frag, _ := ffmpeg.IsFragmentedMP4(part); frag {
		final = filepath.Join(c.dir, key+".m4a")
		remuxed := filepath.Join(c.dir, key+".remux")
		if err := c.remux(ctx, part, remuxed); err != nil {
			// Still serve the original: a real Content-Length at least makes the stream play
			// through, even if the browser's duration/seek bar stays wrong.
			log.Warn(ctx, "Tidal: could not remux fragmented stream; serving it as-is", "id", id, err)
			_ = os.Remove(remuxed)
		} else {
			_ = os.Remove(part)
			part = remuxed
		}
	} else {
		final = filepath.Join(c.dir, key+extForContentType(contentType, part))
	}
	if err := os.Rename(part, final); err != nil {
		_ = os.Remove(part)
		return CachedTrack{}, err
	}
	log.Debug(ctx, "Tidal: cached stream", "id", id, "path", final, "elapsed", time.Since(start))
	go c.prune(final)
	return CachedTrack{Path: final, ContentType: contentTypeByExt[filepath.Ext(final)]}, nil
}

// prune evicts least-recently-used tracks until the cache fits in maxBytes, never evicting keep
// (the track just written, which a request is about to serve).
func (c *streamCache) prune(keep string) {
	if c.maxBytes <= 0 {
		return
	}
	c.pruneMu.Lock()
	defer c.pruneMu.Unlock()
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return
	}
	type entry struct {
		path    string
		size    int64
		modTime time.Time
	}
	var files []entry
	var total int64
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		p := filepath.Join(c.dir, e.Name())
		if ext := filepath.Ext(p); ext == ".part" || ext == ".remux" {
			if time.Since(info.ModTime()) > staleTempAge {
				_ = os.Remove(p)
			}
			continue
		}
		files = append(files, entry{p, info.Size(), info.ModTime()})
		total += info.Size()
	}
	slices.SortFunc(files, func(a, b entry) int { return a.modTime.Compare(b.modTime) })
	for _, f := range files {
		if total <= c.maxBytes {
			break
		}
		if f.path == keep {
			continue
		}
		if err := os.Remove(f.path); err == nil {
			total -= f.size
		}
	}
}
