package downloader

import (
	"github.com/navidrome/navidrome/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("parseProgress", func() {
	It("parses a yt-dlp progress line", func() {
		pct, msg, ok := parseProgress(model.DownloadToolYtDlp,
			"[download]  42.9% of    3.45MiB at    1.20MiB/s ETA 00:02")
		Expect(ok).To(BeTrue())
		Expect(pct).To(BeNumerically("~", 0.429, 0.001))
		Expect(msg).ToNot(BeEmpty())
	})

	It("parses a yt-dlp completion line", func() {
		pct, _, ok := parseProgress(model.DownloadToolYtDlp, "[download] 100% of 3.45MiB in 00:03")
		Expect(ok).To(BeTrue())
		Expect(pct).To(Equal(1.0))
	})

	It("parses a spotdl progress line", func() {
		pct, msg, ok := parseProgress(model.DownloadToolSpotdl, `Downloading "Song Name": 42%`)
		Expect(ok).To(BeTrue())
		Expect(pct).To(BeNumerically("~", 0.42, 0.001))
		Expect(msg).ToNot(BeEmpty())
	})

	It("reports no progress for scdl output", func() {
		_, _, ok := parseProgress(model.DownloadToolScdl, "Downloading the-track.mp3")
		Expect(ok).To(BeFalse())
	})

	It("reports no progress for bandcamp-downloader output", func() {
		_, _, ok := parseProgress(model.DownloadToolBandcampDl, "Downloading album art...")
		Expect(ok).To(BeFalse())
	})

	It("reports no progress for an empty or unmatched line", func() {
		_, _, ok := parseProgress(model.DownloadToolYtDlp, "")
		Expect(ok).To(BeFalse())
		_, _, ok = parseProgress(model.DownloadToolYtDlp, "some unrelated log line")
		Expect(ok).To(BeFalse())
	})
})
