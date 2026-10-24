package downloader

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/auth"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/server/events"
)

const (
	// workerPollInterval is only a fallback: Submit/Retry wake the worker directly.
	workerPollInterval = 30 * time.Second
	// progressMinInterval/progressMinDelta throttle progress writes: yt-dlp alone prints several
	// lines a second, and each one would otherwise cost a DB write and an SSE broadcast.
	progressMinInterval = time.Second
	progressMinDelta    = 0.01
)

// Worker drains the download queue, running up to concurrency jobs at once, dispatched to
// either an external CLI tool (exec_jobs.go) or the Tidal client (tidal_job.go). There is no
// auto-retry - these are large one-shot jobs, not idempotent cheap lookups, so a failure just
// leaves the job in DownloadStatusError for the user to retry. reg lets Service.Cancel stop a
// job that's already running and wakes the worker on new submissions (see registry.go).
type Worker struct {
	ds          model.DataStore
	scanner     model.Scanner
	broker      events.Broker
	tidal       TidalDownloader // nil until Tidal is configured; tidal jobs fail cleanly until then
	reg         *registry
	concurrency int
}

func NewWorker(ds model.DataStore, scanner model.Scanner, broker events.Broker, tidal TidalDownloader, reg *registry) *Worker {
	return &Worker{
		ds:          ds,
		scanner:     scanner,
		broker:      broker,
		tidal:       tidal,
		reg:         reg,
		concurrency: max(1, conf.Server.Downloader.MaxConcurrent),
	}
}

// Run blocks draining the queue until ctx is cancelled. Each free slot claims the next queued
// job as soon as it frees up, so one long download never holds back the rest of the queue.
func (w *Worker) Run(ctx context.Context) error {
	ctx = auth.WithAdminUser(ctx, w.ds)
	// Nothing can still be running at startup: anything left "downloading" was cut off by a crash
	// or restart, and would otherwise sit there forever, uncancelable and never retried.
	if n, err := w.ds.Download(ctx).FailInterrupted("interrupted by a server restart"); err != nil {
		log.Warn(ctx, "Downloader: could not reset interrupted jobs", err)
	} else if n > 0 {
		log.Warn(ctx, "Downloader: marked jobs interrupted by a restart as failed", "count", n)
	}

	ticker := time.NewTicker(workerPollInterval)
	defer ticker.Stop()
	sem := make(chan struct{}, w.concurrency)
	var wg sync.WaitGroup
	defer wg.Wait()
	for {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			return nil
		}
		item, err := w.ds.Download(ctx).ClaimNext()
		if err != nil {
			<-sem
			if !errors.Is(err, model.ErrNotFound) && ctx.Err() == nil {
				log.Warn(ctx, "Downloader: could not claim next job", err)
			}
			select {
			case <-ctx.Done():
				return nil
			case <-w.reg.wake:
			case <-ticker.C:
			}
			continue
		}
		wg.Go(func() {
			defer func() { <-sem }()
			w.process(ctx, *item)
		})
	}
}

// process runs one job the worker has already claimed (ClaimNext moved it to downloading).
func (w *Worker) process(ctx context.Context, item model.Download) {
	log.Info(ctx, "Downloader: starting job", "id", item.ID, "tool", item.Tool, "libraryId", item.LibraryID)
	w.broadcast(ctx, item.ID, model.DownloadStatusDownloading, 0, "", "")

	// jobCtx (not ctx) is what actually runs the download, so a cancellation request or timeout
	// only ever stops this one job - the DB writes below always use ctx, the worker's own
	// long-lived context, so they succeed even after jobCtx has been canceled.
	var jobCtx context.Context
	var cancelJob context.CancelFunc
	if conf.Server.Downloader.JobTimeout > 0 {
		jobCtx, cancelJob = context.WithTimeout(ctx, conf.Server.Downloader.JobTimeout)
	} else {
		jobCtx, cancelJob = context.WithCancel(ctx)
	}
	w.reg.register(item.ID, cancelJob)
	defer func() {
		cancelJob()
		w.reg.unregister(item.ID)
	}()

	targetDir, err := resolveTargetDir(ctx, w.ds, &item)
	if err != nil {
		w.fail(ctx, item.ID, err)
		return
	}

	onProgress := w.progressReporter(ctx, item.ID)

	var moved int
	var lastPath string
	if item.Tool == model.DownloadToolTidal {
		moved, lastPath, err = w.runTidalJob(jobCtx, &item, targetDir, onProgress)
	} else {
		staging := stagingDirFor(&item)
		if err = runExecJob(jobCtx, &item, staging, onProgress); err == nil {
			moved, lastPath, err = moveJobOutput(staging, targetDir)
		} else {
			_ = os.RemoveAll(staging)
		}
	}
	if err != nil {
		switch {
		case errors.Is(jobCtx.Err(), context.Canceled):
			// A user-initiated cancel (registry.cancel) always uses plain context.Canceled,
			// whether jobCtx itself is a WithCancel or a WithTimeout context.
			w.markCanceled(ctx, item.ID)
		case errors.Is(jobCtx.Err(), context.DeadlineExceeded):
			w.fail(ctx, item.ID, fmt.Errorf("timed out after %s", conf.Server.Downloader.JobTimeout))
		default:
			w.fail(ctx, item.ID, err)
		}
		return
	}
	if moved == 0 {
		w.fail(ctx, item.ID, errors.New("no files produced"))
		return
	}

	// A multi-file job (an album, a playlist) is represented by the folder it landed in.
	result := lastPath
	if moved > 1 {
		result = filepath.Dir(lastPath)
	}
	if err := w.ds.Download(ctx).MarkCompleted(item.ID, result); err != nil {
		log.Warn(ctx, "Downloader: could not mark job completed", "id", item.ID, err)
	}
	log.Info(ctx, "Downloader: job completed", "id", item.ID, "files", moved, "path", result)
	w.broadcast(ctx, item.ID, model.DownloadStatusCompleted, 1, "", "")
	w.triggerRescan(ctx, item.LibraryID, filepath.Dir(lastPath))
}

// progressReporter returns the onProgress callback for one job, persisting and broadcasting
// only meaningful changes (see progressMinInterval/progressMinDelta).
func (w *Worker) progressReporter(ctx context.Context, id string) func(pct float64, msg string) {
	var mu sync.Mutex
	lastPct := -1.0
	var lastAt time.Time
	return func(pct float64, msg string) {
		mu.Lock()
		if pct-lastPct < progressMinDelta && time.Since(lastAt) < progressMinInterval {
			mu.Unlock()
			return
		}
		lastPct, lastAt = pct, time.Now()
		mu.Unlock()
		if e := w.ds.Download(ctx).UpdateProgress(id, pct, msg); e != nil {
			log.Warn(ctx, "Downloader: could not update job progress", "id", id, e)
		}
		w.broadcast(ctx, id, model.DownloadStatusDownloading, pct, msg, "")
	}
}

func (w *Worker) fail(ctx context.Context, id string, err error) {
	log.Error(ctx, "Downloader: job failed", "id", id, err)
	msg := err.Error()
	if e := w.ds.Download(ctx).MarkFailed(id, msg); e != nil {
		log.Warn(ctx, "Downloader: could not mark job failed", "id", id, e)
	}
	w.broadcast(ctx, id, model.DownloadStatusError, 0, "", msg)
}

func (w *Worker) markCanceled(ctx context.Context, id string) {
	log.Info(ctx, "Downloader: job canceled", "id", id)
	if e := w.ds.Download(ctx).MarkCanceled(id); e != nil {
		log.Warn(ctx, "Downloader: could not mark job canceled", "id", id, e)
	}
	w.broadcast(ctx, id, model.DownloadStatusCanceled, 0, "", "")
}

func (w *Worker) broadcast(ctx context.Context, id string, status model.DownloadStatus, pct float64, msg, errMsg string) {
	w.broker.SendBroadcastMessage(ctx, &events.DownloadStatus{
		ID: id, Status: string(status), Progress: pct, StatusMessage: msg, Error: errMsg,
	})
}

// triggerRescan scans just the folder the job wrote into, so the new tracks show up without
// waiting for (or paying the cost of) a full library scan.
func (w *Worker) triggerRescan(ctx context.Context, libraryID int, absFolderPath string) {
	libPath, err := w.ds.Library(ctx).GetPath(libraryID)
	if err != nil {
		log.Warn(ctx, "Downloader: could not resolve library path for rescan", "libraryId", libraryID, err)
		return
	}
	rel, err := filepath.Rel(libPath, absFolderPath)
	if err != nil {
		rel = "."
	}
	if _, err := w.scanner.ScanFolders(ctx, false, []model.ScanTarget{{LibraryID: libraryID, FolderPath: rel}}); err != nil {
		log.Warn(ctx, "Downloader: post-download rescan failed", "libraryId", libraryID, "folder", rel, err)
	}
}
