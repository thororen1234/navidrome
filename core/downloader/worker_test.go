package downloader

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/server/events"
	"github.com/navidrome/navidrome/tests"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type fakeEventBroker struct {
	http.Handler
	mu     sync.Mutex
	events []events.Event
}

func (f *fakeEventBroker) SendMessage(_ context.Context, event events.Event) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, event)
}

func (f *fakeEventBroker) SendBroadcastMessage(_ context.Context, event events.Event) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, event)
}

func (f *fakeEventBroker) getEvents() []events.Event {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]events.Event(nil), f.events...)
}

var _ events.Broker = (*fakeEventBroker)(nil)

type fakeTidal struct {
	content []byte
	name    string
	err     error
	started chan struct{} // closed once DownloadTo begins, so a test can synchronize before canceling
	block   chan struct{} // if non-nil, DownloadTo waits on this (or ctx.Done()) before returning
}

func (f *fakeTidal) DownloadTo(ctx context.Context, _, _ string, w io.Writer) (string, error) {
	if f.started != nil {
		close(f.started)
	}
	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	if f.err != nil {
		return "", f.err
	}
	_, err := w.Write(f.content)
	return f.name, err
}

func zipOf(files map[string]string) []byte {
	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)
	for name, content := range files {
		w, _ := zw.Create(name)
		_, _ = w.Write([]byte(content))
	}
	_ = zw.Close()
	return buf.Bytes()
}

var _ = Describe("Worker", func() {
	var ds *tests.MockDataStore
	var scanner *tests.MockScanner
	var broker *fakeEventBroker
	var reg *registry
	var libPath string
	var ctx context.Context

	BeforeEach(func() {
		libPath = GinkgoT().TempDir()
		ds = &tests.MockDataStore{
			MockedLibrary: &tests.MockLibraryRepo{Data: map[int]model.Library{1: {ID: 1, Path: libPath}}},
		}
		scanner = tests.NewMockScanner()
		broker = &fakeEventBroker{}
		reg = NewRegistry()
		ctx = context.Background()
	})

	newDownload := func(tool model.DownloadTool, kind string) model.Download {
		d := model.Download{Tool: tool, TidalKind: kind, LibraryID: 1, RequestedBy: "admin"}
		Expect(ds.Download(ctx).Enqueue(&d)).To(Succeed())
		return d
	}

	It("completes a tidal track job and triggers a targeted rescan", func() {
		w := NewWorker(ds, scanner, broker, &fakeTidal{content: []byte("audio-bytes"), name: "Song.flac"}, reg)
		d := newDownload(model.DownloadToolTidal, "track")

		w.process(ctx, d)

		got, err := ds.Download(ctx).Get(d.ID)
		Expect(err).ToNot(HaveOccurred())
		Expect(got.Status).To(Equal(model.DownloadStatusCompleted))
		Expect(got.TargetPath).To(HavePrefix(filepath.Join(libPath, "Downloads", "tidal")))
		Expect(got.TargetPath).To(BeAnExistingFile())

		content, err := os.ReadFile(got.TargetPath)
		Expect(err).ToNot(HaveOccurred())
		Expect(string(content)).To(Equal("audio-bytes"))

		Expect(scanner.GetScanFoldersCallCount()).To(Equal(1))
		calls := scanner.GetScanFoldersCalls()
		Expect(calls[0].Targets).To(HaveLen(1))
		Expect(calls[0].Targets[0].LibraryID).To(Equal(1))
		Expect(calls[0].Targets[0].FolderPath).To(Equal(filepath.Join("Downloads", "tidal")))

		var sawCompleted bool
		for _, e := range broker.getEvents() {
			if evt, ok := e.(*events.DownloadStatus); ok && evt.Status == string(model.DownloadStatusCompleted) {
				sawCompleted = true
			}
		}
		Expect(sawCompleted).To(BeTrue())
	})

	It("extracts every file from a tidal album zip", func() {
		zipBytes := zipOf(map[string]string{"01 Track.flac": "one", "02 Track.flac": "two"})
		w := NewWorker(ds, scanner, broker, &fakeTidal{content: zipBytes}, reg)
		d := newDownload(model.DownloadToolTidal, "album")

		w.process(ctx, d)

		got, err := ds.Download(ctx).Get(d.ID)
		Expect(err).ToNot(HaveOccurred())
		Expect(got.Status).To(Equal(model.DownloadStatusCompleted))

		targetDir := filepath.Join(libPath, "Downloads", "tidal")
		Expect(filepath.Join(targetDir, "01 Track.flac")).To(BeAnExistingFile())
		Expect(filepath.Join(targetDir, "02 Track.flac")).To(BeAnExistingFile())
	})

	It("marks the job failed when the tidal client errors", func() {
		w := NewWorker(ds, scanner, broker, &fakeTidal{err: errors.New("boom")}, reg)
		d := newDownload(model.DownloadToolTidal, "track")

		w.process(ctx, d)

		got, err := ds.Download(ctx).Get(d.ID)
		Expect(err).ToNot(HaveOccurred())
		Expect(got.Status).To(Equal(model.DownloadStatusError))
		Expect(got.Error).To(ContainSubstring("boom"))
		Expect(scanner.GetScanFoldersCallCount()).To(Equal(0))
	})

	It("fails cleanly when tidal is not configured", func() {
		w := NewWorker(ds, scanner, broker, nil, reg)
		d := newDownload(model.DownloadToolTidal, "track")

		w.process(ctx, d)

		got, err := ds.Download(ctx).Get(d.ID)
		Expect(err).ToNot(HaveOccurred())
		Expect(got.Status).To(Equal(model.DownloadStatusError))
		Expect(got.Error).To(ContainSubstring("not configured"))
	})

	It("cancels a running job via the registry instead of letting it fail", func() {
		started := make(chan struct{})
		block := make(chan struct{})
		w := NewWorker(ds, scanner, broker, &fakeTidal{started: started, block: block}, reg)
		d := newDownload(model.DownloadToolTidal, "track")

		done := make(chan struct{})
		go func() {
			defer close(done)
			w.process(ctx, d)
		}()

		Eventually(started).Should(BeClosed())
		Expect(reg.cancel(d.ID)).To(BeTrue(), "job should still be registered as running")
		Eventually(done).Should(BeClosed())

		got, err := ds.Download(ctx).Get(d.ID)
		Expect(err).ToNot(HaveOccurred())
		Expect(got.Status).To(Equal(model.DownloadStatusCanceled))

		var sawCanceled bool
		for _, e := range broker.getEvents() {
			if evt, ok := e.(*events.DownloadStatus); ok && evt.Status == string(model.DownloadStatusCanceled) {
				sawCanceled = true
			}
		}
		Expect(sawCanceled).To(BeTrue())
	})

	It("marks a job that exceeds JobTimeout as failed, not canceled", func() {
		prev := conf.Server.Downloader.JobTimeout
		conf.Server.Downloader.JobTimeout = 20 * time.Millisecond
		defer func() { conf.Server.Downloader.JobTimeout = prev }()

		block := make(chan struct{}) // never closed: DownloadTo only returns via ctx timeout
		w := NewWorker(ds, scanner, broker, &fakeTidal{block: block}, reg)
		d := newDownload(model.DownloadToolTidal, "track")

		w.process(ctx, d)

		got, err := ds.Download(ctx).Get(d.ID)
		Expect(err).ToNot(HaveOccurred())
		Expect(got.Status).To(Equal(model.DownloadStatusError))
		Expect(got.Error).To(ContainSubstring("timed out"))
	})

	It("reports the job as no longer running once it has finished", func() {
		w := NewWorker(ds, scanner, broker, &fakeTidal{content: []byte("x"), name: "x.flac"}, reg)
		d := newDownload(model.DownloadToolTidal, "track")

		w.process(ctx, d)

		Expect(reg.cancel(d.ID)).To(BeFalse(), "a finished job should already be unregistered")
	})

	It("fails when the target library no longer exists", func() {
		w := NewWorker(ds, scanner, broker, &fakeTidal{content: []byte("x"), name: "x.flac"}, reg)
		d := model.Download{Tool: model.DownloadToolTidal, TidalKind: "track", LibraryID: 99, RequestedBy: "admin"}
		Expect(ds.Download(ctx).Enqueue(&d)).To(Succeed())

		w.process(ctx, d)

		got, err := ds.Download(ctx).Get(d.ID)
		Expect(err).ToNot(HaveOccurred())
		Expect(got.Status).To(Equal(model.DownloadStatusError))
	})
})
