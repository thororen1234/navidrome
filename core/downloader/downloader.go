// Package downloader implements the Downloader tab's job queue: submitting yt-dlp/scdl/spotdl/
// bandcamp-dl/Tidal jobs, and a background worker that executes them into a library folder.
package downloader

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/navidrome/navidrome/model"
)

var (
	ErrInvalidRequest = errors.New("invalid download request")
)

// SubmitRequest describes a new job. For tool-based jobs (yt-dlp/scdl/spotdl/bandcamp-dl),
// SourceURL is required. For Tool == model.DownloadToolTidal, TidalID and TidalKind are
// required instead.
type SubmitRequest struct {
	Tool      model.DownloadTool
	SourceURL string
	TidalID   string
	TidalKind string
	LibraryID int
	UserID    string
}

// Service is the synchronous half of the downloader: it validates and enqueues jobs. The
// Worker drains the queue asynchronously.
type Service interface {
	Submit(ctx context.Context, req SubmitRequest) (*model.Download, error)
	Cancel(ctx context.Context, id string) error
}

type service struct {
	ds model.DataStore
}

func NewService(ds model.DataStore) Service {
	return &service{ds: ds}
}

func (s *service) Submit(ctx context.Context, req SubmitRequest) (*model.Download, error) {
	if err := validate(req); err != nil {
		return nil, err
	}
	if _, err := s.ds.Library(ctx).Get(req.LibraryID); err != nil {
		return nil, fmt.Errorf("%w: library %d: %w", ErrInvalidRequest, req.LibraryID, err)
	}
	d := &model.Download{
		Tool:        req.Tool,
		SourceURL:   req.SourceURL,
		TidalID:     req.TidalID,
		TidalKind:   req.TidalKind,
		LibraryID:   req.LibraryID,
		RequestedBy: req.UserID,
	}
	if err := s.ds.Download(ctx).Enqueue(d); err != nil {
		return nil, err
	}
	return d, nil
}

func (s *service) Cancel(ctx context.Context, id string) error {
	d, err := s.ds.Download(ctx).Get(id)
	if err != nil {
		return err
	}
	if d.Status != model.DownloadStatusQueued {
		return fmt.Errorf("%w: job %s is %s, not queued", ErrInvalidRequest, id, d.Status)
	}
	return s.ds.Download(ctx).MarkCanceled(id)
}

func validate(req SubmitRequest) error {
	if req.LibraryID == 0 {
		return fmt.Errorf("%w: libraryId is required", ErrInvalidRequest)
	}
	if req.Tool == model.DownloadToolTidal {
		if req.TidalID == "" {
			return fmt.Errorf("%w: tidalId is required for tidal downloads", ErrInvalidRequest)
		}
		if req.TidalKind != "track" && req.TidalKind != "album" {
			return fmt.Errorf("%w: tidalKind must be 'track' or 'album'", ErrInvalidRequest)
		}
		return nil
	}
	switch req.Tool {
	case model.DownloadToolYtDlp, model.DownloadToolScdl, model.DownloadToolSpotdl, model.DownloadToolBandcampDl:
	default:
		return fmt.Errorf("%w: unknown tool %q", ErrInvalidRequest, req.Tool)
	}
	if !strings.HasPrefix(req.SourceURL, "http://") && !strings.HasPrefix(req.SourceURL, "https://") {
		// Also closes off argument injection: every tool is invoked with the URL as a bare argv
		// element, so a value starting with "-" could otherwise be parsed as a flag.
		return fmt.Errorf("%w: sourceUrl must be an http(s) URL", ErrInvalidRequest)
	}
	return nil
}
