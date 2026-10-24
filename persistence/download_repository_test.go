package persistence

import (
	"context"

	"github.com/Masterminds/squirrel"
	"github.com/navidrome/navidrome/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("DownloadRepository", func() {
	var repo model.DownloadRepository
	ctx := context.Background()

	job := func(tool model.DownloadTool, url string) *model.Download {
		return &model.Download{Tool: tool, SourceURL: url, LibraryID: 1, RequestedBy: "admin"}
	}

	clearDownloads := func() {
		r := repo.(*downloadRepository)
		_, err := r.executeSQL(ctx, squirrel.Delete(r.tableName))
		Expect(err).ToNot(HaveOccurred())
	}

	BeforeEach(func() {
		repo = NewDownloadRepository(GetDBXBuilder())
		clearDownloads()
		DeferCleanup(clearDownloads)
	})

	It("enqueues a job as queued and assigns an id", func() {
		d := job(model.DownloadToolYtDlp, "https://example.com/video")
		Expect(repo.Enqueue(ctx, d)).To(Succeed())
		Expect(d.ID).ToNot(BeEmpty())
		Expect(d.Status).To(Equal(model.DownloadStatusQueued))

		got, err := repo.Get(ctx, d.ID)
		Expect(err).ToNot(HaveOccurred())
		Expect(got.Tool).To(Equal(model.DownloadToolYtDlp))
		Expect(got.SourceURL).To(Equal("https://example.com/video"))
		Expect(got.Status).To(Equal(model.DownloadStatusQueued))
	})

	It("returns ErrNotFound for a missing id", func() {
		_, err := repo.Get(ctx, "does-not-exist")
		Expect(err).To(MatchError(model.ErrNotFound))
	})

	It("claims only queued jobs, oldest first, one at a time", func() {
		d1 := job(model.DownloadToolYtDlp, "https://example.com/1")
		Expect(repo.Enqueue(ctx, d1)).To(Succeed())
		d2 := job(model.DownloadToolScdl, "https://example.com/2")
		Expect(repo.Enqueue(ctx, d2)).To(Succeed())
		d3 := job(model.DownloadToolScdl, "https://example.com/3")
		Expect(repo.Enqueue(ctx, d3)).To(Succeed())
		Expect(repo.MarkCompleted(ctx, d1.ID, "/music/1.mp3")).To(Succeed())

		got, err := repo.ClaimNext(ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(got.ID).To(Equal(d2.ID))
		Expect(got.Status).To(Equal(model.DownloadStatusDownloading))
		Expect(got.StartedAt).ToNot(BeNil())

		got, err = repo.ClaimNext(ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(got.ID).To(Equal(d3.ID))

		_, err = repo.ClaimNext(ctx)
		Expect(err).To(MatchError(model.ErrNotFound))
	})

	It("transitions through progress, completion and failure", func() {
		d := job(model.DownloadToolSpotdl, "https://example.com/track")
		Expect(repo.Enqueue(ctx, d)).To(Succeed())

		_, err := repo.ClaimNext(ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(repo.UpdateProgress(ctx, d.ID, 0.5, "downloading: 50%")).To(Succeed())

		got, err := repo.Get(ctx, d.ID)
		Expect(err).ToNot(HaveOccurred())
		Expect(got.Status).To(Equal(model.DownloadStatusDownloading))
		Expect(got.Progress).To(Equal(0.5))
		Expect(got.StatusMessage).To(Equal("downloading: 50%"))
		Expect(got.StartedAt).ToNot(BeNil())

		Expect(repo.MarkCompleted(ctx, d.ID, "/music/track.mp3")).To(Succeed())
		got, err = repo.Get(ctx, d.ID)
		Expect(err).ToNot(HaveOccurred())
		Expect(got.Status).To(Equal(model.DownloadStatusCompleted))
		Expect(got.Progress).To(Equal(1.0))
		Expect(got.TargetPath).To(Equal("/music/track.mp3"))
		Expect(got.CompletedAt).ToNot(BeNil())
	})

	It("marks a job failed and increments attempts", func() {
		d := job(model.DownloadToolBandcampDl, "https://example.com/album")
		Expect(repo.Enqueue(ctx, d)).To(Succeed())

		Expect(repo.MarkFailed(ctx, d.ID, "exit status 1")).To(Succeed())
		got, err := repo.Get(ctx, d.ID)
		Expect(err).ToNot(HaveOccurred())
		Expect(got.Status).To(Equal(model.DownloadStatusError))
		Expect(got.Error).To(Equal("exit status 1"))
		Expect(got.Attempts).To(Equal(1))
	})

	It("cancels a queued job so it is never claimed", func() {
		d := job(model.DownloadToolYtDlp, "https://example.com/cancel-me")
		Expect(repo.Enqueue(ctx, d)).To(Succeed())

		ok, err := repo.CancelQueued(ctx, d.ID)
		Expect(err).ToNot(HaveOccurred())
		Expect(ok).To(BeTrue())
		got, err := repo.Get(ctx, d.ID)
		Expect(err).ToNot(HaveOccurred())
		Expect(got.Status).To(Equal(model.DownloadStatusCanceled))

		_, err = repo.ClaimNext(ctx)
		Expect(err).To(MatchError(model.ErrNotFound))
	})

	It("does not cancel-as-queued a job the worker already claimed", func() {
		d := job(model.DownloadToolYtDlp, "https://example.com/racy")
		Expect(repo.Enqueue(ctx, d)).To(Succeed())
		_, err := repo.ClaimNext(ctx)
		Expect(err).ToNot(HaveOccurred())

		ok, err := repo.CancelQueued(ctx, d.ID)
		Expect(err).ToNot(HaveOccurred())
		Expect(ok).To(BeFalse())
		got, _ := repo.Get(ctx, d.ID)
		Expect(got.Status).To(Equal(model.DownloadStatusDownloading))
	})

	It("requeues failed and canceled jobs, but not running or completed ones", func() {
		failed := job(model.DownloadToolYtDlp, "https://example.com/failed")
		Expect(repo.Enqueue(ctx, failed)).To(Succeed())
		Expect(repo.MarkFailed(ctx, failed.ID, "boom")).To(Succeed())
		done := job(model.DownloadToolYtDlp, "https://example.com/done")
		Expect(repo.Enqueue(ctx, done)).To(Succeed())
		Expect(repo.MarkCompleted(ctx, done.ID, "/music/x.mp3")).To(Succeed())

		ok, err := repo.Requeue(ctx, failed.ID)
		Expect(err).ToNot(HaveOccurred())
		Expect(ok).To(BeTrue())
		got, _ := repo.Get(ctx, failed.ID)
		Expect(got.Status).To(Equal(model.DownloadStatusQueued))
		Expect(got.Error).To(BeEmpty())
		Expect(got.Attempts).To(Equal(1), "attempts survive a retry")

		ok, err = repo.Requeue(ctx, done.ID)
		Expect(err).ToNot(HaveOccurred())
		Expect(ok).To(BeFalse())
	})

	It("fails jobs left downloading by a previous run", func() {
		d := job(model.DownloadToolYtDlp, "https://example.com/orphan")
		Expect(repo.Enqueue(ctx, d)).To(Succeed())
		_, err := repo.ClaimNext(ctx)
		Expect(err).ToNot(HaveOccurred())
		queued := job(model.DownloadToolYtDlp, "https://example.com/queued")
		Expect(repo.Enqueue(ctx, queued)).To(Succeed())

		n, err := repo.FailInterrupted(ctx, "interrupted")
		Expect(err).ToNot(HaveOccurred())
		Expect(n).To(Equal(int64(1)))
		got, _ := repo.Get(ctx, d.ID)
		Expect(got.Status).To(Equal(model.DownloadStatusError))
		Expect(got.Error).To(Equal("interrupted"))
		got, _ = repo.Get(ctx, queued.ID)
		Expect(got.Status).To(Equal(model.DownloadStatusQueued))
	})

	It("deletes only finished jobs", func() {
		queued := job(model.DownloadToolYtDlp, "https://example.com/q")
		Expect(repo.Enqueue(ctx, queued)).To(Succeed())
		for _, mark := range []func(string) error{
			func(id string) error { return repo.MarkCompleted(ctx, id, "/x") },
			func(id string) error { return repo.MarkFailed(ctx, id, "boom") },
			func(id string) error { return repo.MarkCanceled(ctx, id) },
		} {
			d := job(model.DownloadToolYtDlp, "https://example.com/f")
			Expect(repo.Enqueue(ctx, d)).To(Succeed())
			Expect(mark(d.ID)).To(Succeed())
		}

		n, err := repo.DeleteFinished(ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(n).To(Equal(int64(3)))
		all, err := repo.GetAll(ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(all).To(HaveLen(1))
		Expect(all[0].ID).To(Equal(queued.ID))
	})

	It("counts jobs by status", func() {
		Expect(repo.Enqueue(ctx, job(model.DownloadToolYtDlp, "https://example.com/a"))).To(Succeed())
		Expect(repo.Enqueue(ctx, job(model.DownloadToolYtDlp, "https://example.com/b"))).To(Succeed())
		failed := job(model.DownloadToolYtDlp, "https://example.com/c")
		Expect(repo.Enqueue(ctx, failed)).To(Succeed())
		Expect(repo.MarkFailed(ctx, failed.ID, "boom")).To(Succeed())

		queuedCount, err := repo.CountByStatus(ctx, model.DownloadStatusQueued)
		Expect(err).ToNot(HaveOccurred())
		Expect(queuedCount).To(Equal(int64(2)))

		errorCount, err := repo.CountByStatus(ctx, model.DownloadStatusError)
		Expect(err).ToNot(HaveOccurred())
		Expect(errorCount).To(Equal(int64(1)))
	})

	It("lists all jobs via GetAll", func() {
		Expect(repo.Enqueue(ctx, job(model.DownloadToolYtDlp, "https://example.com/a"))).To(Succeed())
		Expect(repo.Enqueue(ctx, job(model.DownloadToolScdl, "https://example.com/b"))).To(Succeed())

		all, err := repo.GetAll(ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(all).To(HaveLen(2))
	})
})
