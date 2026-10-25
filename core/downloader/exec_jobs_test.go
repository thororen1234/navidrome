package downloader

import (
	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/conf/configtest"
	"github.com/navidrome/navidrome/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("errorDetail", func() {
	It("reports yt-dlp's ERROR line instead of the warnings before it", func() {
		stderr := `WARNING: [youtube] No supported JavaScript runtime could be found. Only deno is enabled by default
WARNING: [youtube] No title found in player responses; falling back to title from initial data.
ERROR: [youtube] rA3--S70iXg: Sign in to confirm you're not a bot. Use --cookies-from-browser or --cookies for the authentication.
`
		Expect(errorDetail(stderr)).To(Equal(
			"ERROR: [youtube] rA3--S70iXg: Sign in to confirm you're not a bot. Use --cookies-from-browser or --cookies for the authentication."))
	})

	It("falls back to the whole output when there is no ERROR line", func() {
		Expect(errorDetail("  something went wrong\n")).To(Equal("something went wrong"))
	})
})

var _ = Describe("buildArgs", func() {
	BeforeEach(func() {
		DeferCleanup(configtest.SetupConfig())
	})

	It("passes the configured cookies file to yt-dlp, before the URL", func() {
		conf.Server.Downloader.YtDlpCookiesFile = "/data/cookies.txt"
		args := buildArgs(model.DownloadToolYtDlp, "yt-dlp", "https://music.youtube.com/watch?v=x", "/tmp/out")
		Expect(args).To(ContainElements("--cookies", "/data/cookies.txt"))
		Expect(args[len(args)-1]).To(Equal("https://music.youtube.com/watch?v=x"))
	})

	It("adds no cookies flag when none is configured", func() {
		args := buildArgs(model.DownloadToolYtDlp, "yt-dlp", "https://music.youtube.com/watch?v=x", "/tmp/out")
		Expect(args).ToNot(ContainElement("--cookies"))
	})
})
