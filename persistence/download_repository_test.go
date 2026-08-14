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

	job := func(tool model.DownloadTool, url string) *model.Download {
		return &model.Download{Tool: tool, SourceURL: url, LibraryID: 1, RequestedBy: "admin"}
	}

	clearDownloads := func() {
		r := repo.(*downloadRepository)
		_, err := r.executeSQL(squirrel.Delete(r.tableName))
		Expect(err).ToNot(HaveOccurred())
	}

	BeforeEach(func() {
		repo = NewDownloadRepository(context.Background(), GetDBXBuilder())
		clearDownloads()
		DeferCleanup(clearDownloads)
	})

	It("enqueues a job as queued and assigns an id", func() {
		d := job(model.DownloadToolYtDlp, "https://example.com/video")
		Expect(repo.Enqueue(d)).To(Succeed())
		Expect(d.ID).ToNot(BeEmpty())
		Expect(d.Status).To(Equal(model.DownloadStatusQueued))

		got, err := repo.Get(d.ID)
		Expect(err).ToNot(HaveOccurred())
		Expect(got.Tool).To(Equal(model.DownloadToolYtDlp))
		Expect(got.SourceURL).To(Equal("https://example.com/video"))
		Expect(got.Status).To(Equal(model.DownloadStatusQueued))
	})

	It("returns ErrNotFound for a missing id", func() {
		_, err := repo.Get("does-not-exist")
		Expect(err).To(MatchError(model.ErrNotFound))
	})

	It("dequeues only queued jobs, oldest first", func() {
		d1 := job(model.DownloadToolYtDlp, "https://example.com/1")
		Expect(repo.Enqueue(d1)).To(Succeed())
		d2 := job(model.DownloadToolScdl, "https://example.com/2")
		Expect(repo.Enqueue(d2)).To(Succeed())
		Expect(repo.MarkCompleted(d1.ID, "/music/1.mp3")).To(Succeed())

		got, err := repo.DequeueBatch(10)
		Expect(err).ToNot(HaveOccurred())
		Expect(got).To(HaveLen(1))
		Expect(got[0].ID).To(Equal(d2.ID))
	})

	It("transitions through progress, completion and failure", func() {
		d := job(model.DownloadToolSpotdl, "https://example.com/track")
		Expect(repo.Enqueue(d)).To(Succeed())

		Expect(repo.MarkStarted(d.ID)).To(Succeed())
		Expect(repo.UpdateProgress(d.ID, 0.5, "downloading: 50%")).To(Succeed())

		got, err := repo.Get(d.ID)
		Expect(err).ToNot(HaveOccurred())
		Expect(got.Status).To(Equal(model.DownloadStatusDownloading))
		Expect(got.Progress).To(Equal(0.5))
		Expect(got.StatusMessage).To(Equal("downloading: 50%"))
		Expect(got.StartedAt).ToNot(BeNil())

		Expect(repo.MarkCompleted(d.ID, "/music/track.mp3")).To(Succeed())
		got, err = repo.Get(d.ID)
		Expect(err).ToNot(HaveOccurred())
		Expect(got.Status).To(Equal(model.DownloadStatusCompleted))
		Expect(got.Progress).To(Equal(1.0))
		Expect(got.TargetPath).To(Equal("/music/track.mp3"))
		Expect(got.CompletedAt).ToNot(BeNil())
	})

	It("marks a job failed and increments attempts", func() {
		d := job(model.DownloadToolBandcampDl, "https://example.com/album")
		Expect(repo.Enqueue(d)).To(Succeed())

		Expect(repo.MarkFailed(d.ID, "exit status 1")).To(Succeed())
		got, err := repo.Get(d.ID)
		Expect(err).ToNot(HaveOccurred())
		Expect(got.Status).To(Equal(model.DownloadStatusError))
		Expect(got.Error).To(Equal("exit status 1"))
		Expect(got.Attempts).To(Equal(1))
	})

	It("cancels a queued job", func() {
		d := job(model.DownloadToolYtDlp, "https://example.com/cancel-me")
		Expect(repo.Enqueue(d)).To(Succeed())

		Expect(repo.MarkCanceled(d.ID)).To(Succeed())
		got, err := repo.Get(d.ID)
		Expect(err).ToNot(HaveOccurred())
		Expect(got.Status).To(Equal(model.DownloadStatusCanceled))

		batch, err := repo.DequeueBatch(10)
		Expect(err).ToNot(HaveOccurred())
		Expect(batch).To(BeEmpty())
	})

	It("counts jobs by status", func() {
		Expect(repo.Enqueue(job(model.DownloadToolYtDlp, "https://example.com/a"))).To(Succeed())
		Expect(repo.Enqueue(job(model.DownloadToolYtDlp, "https://example.com/b"))).To(Succeed())
		failed := job(model.DownloadToolYtDlp, "https://example.com/c")
		Expect(repo.Enqueue(failed)).To(Succeed())
		Expect(repo.MarkFailed(failed.ID, "boom")).To(Succeed())

		queuedCount, err := repo.CountByStatus(model.DownloadStatusQueued)
		Expect(err).ToNot(HaveOccurred())
		Expect(queuedCount).To(Equal(int64(2)))

		errorCount, err := repo.CountByStatus(model.DownloadStatusError)
		Expect(err).ToNot(HaveOccurred())
		Expect(errorCount).To(Equal(int64(1)))
	})

	It("lists all jobs via GetAll", func() {
		Expect(repo.Enqueue(job(model.DownloadToolYtDlp, "https://example.com/a"))).To(Succeed())
		Expect(repo.Enqueue(job(model.DownloadToolScdl, "https://example.com/b"))).To(Succeed())

		all, err := repo.GetAll()
		Expect(err).ToNot(HaveOccurred())
		Expect(all).To(HaveLen(2))
	})
})
