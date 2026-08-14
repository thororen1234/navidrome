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

const workerPollInterval = 5 * time.Second

// Worker drains the download queue: one job at a time per concurrency slot, dispatched to
// either an external CLI tool (exec_jobs.go) or the Tidal client (tidal_job.go). There is no
// auto-retry - these are large one-shot jobs, not idempotent cheap lookups, so a failure just
// leaves the job in DownloadStatusError for the user to resubmit. reg lets Service.Cancel stop a
// job that's already running (see registry.go).
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

// Run blocks draining the queue until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) error {
	ctx = auth.WithAdminUser(ctx, w.ds)
	ticker := time.NewTicker(workerPollInterval)
	defer ticker.Stop()
	for {
		n, err := w.drain(ctx)
		if err != nil && ctx.Err() == nil {
			log.Warn(ctx, "Downloader: worker drain failed", err)
		}
		if ctx.Err() != nil {
			return nil
		}
		if n > 0 {
			continue
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (w *Worker) drain(ctx context.Context) (int, error) {
	items, err := w.ds.Download(ctx).DequeueBatch(max(4, 2*w.concurrency))
	if err != nil {
		return 0, err
	}
	if len(items) == 0 {
		return 0, nil
	}
	sem := make(chan struct{}, w.concurrency)
	var wg sync.WaitGroup
	for _, item := range items {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
		}
		if ctx.Err() != nil {
			wg.Wait()
			return len(items), nil //nolint:nilerr // a cancelled drain is a clean stop, not an error
		}
		wg.Go(func() {
			defer func() { <-sem }()
			w.process(ctx, item)
		})
	}
	wg.Wait()
	return len(items), nil
}

func (w *Worker) process(ctx context.Context, item model.Download) {
	log.Info(ctx, "Downloader: starting job", "id", item.ID, "tool", item.Tool, "libraryId", item.LibraryID)
	if err := w.ds.Download(ctx).MarkStarted(item.ID); err != nil {
		log.Warn(ctx, "Downloader: could not mark job started", "id", item.ID, err)
	}
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

	onProgress := func(pct float64, msg string) {
		if e := w.ds.Download(ctx).UpdateProgress(item.ID, pct, msg); e != nil {
			log.Warn(ctx, "Downloader: could not update job progress", "id", item.ID, e)
		}
		w.broadcast(ctx, item.ID, model.DownloadStatusDownloading, pct, msg, "")
	}

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

	result := lastPath
	if moved > 1 {
		result = targetDir
	}
	if err := w.ds.Download(ctx).MarkCompleted(item.ID, result); err != nil {
		log.Warn(ctx, "Downloader: could not mark job completed", "id", item.ID, err)
	}
	w.broadcast(ctx, item.ID, model.DownloadStatusCompleted, 1, "", "")
	w.triggerRescan(ctx, item.LibraryID, targetDir)
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
