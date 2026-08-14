package toolmgr

import (
	"context"
	"testing"

	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestToolmgr(t *testing.T) {
	log.SetLevel(log.LevelFatal)
	RegisterFailHandler(Fail)
	RunSpecs(t, "Toolmgr Suite")
}

var _ = Describe("argsForAction", func() {
	DescribeTable("pip mode",
		func(action Action, expected []string) {
			Expect(argsForAction(action, "yt-dlp", false)).To(Equal(expected))
		},
		Entry("install", ActionInstall, []string{"install", "-U", "yt-dlp"}),
		Entry("upgrade", ActionUpgrade, []string{"install", "-U", "yt-dlp"}),
		Entry("repair", ActionRepair, []string{"install", "--force-reinstall", "yt-dlp"}),
	)

	DescribeTable("pipx mode",
		func(action Action, expected []string) {
			Expect(argsForAction(action, "yt-dlp", true)).To(Equal(expected))
		},
		Entry("install", ActionInstall, []string{"install", "yt-dlp"}),
		Entry("upgrade", ActionUpgrade, []string{"upgrade", "yt-dlp"}),
		Entry("repair", ActionRepair, []string{"reinstall", "yt-dlp"}),
	)

	It("never derives the package name from anything but the fixed lookup table", func() {
		for tool, pkg := range pipPackages {
			args := argsForAction(ActionInstall, pkg, false)
			Expect(args).To(ContainElement(pkg), "tool %s", tool)
		}
	})

	It("returns nil for an unknown action", func() {
		Expect(argsForAction(Action("bogus"), "yt-dlp", false)).To(BeNil())
	})
})

var _ = Describe("Run", func() {
	It("rejects a tool with no known pip package", func() {
		m := New()
		err := m.Run(context.Background(), model.DownloadToolTidal, ActionInstall)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unknown downloader tool"))
	})
})

var _ = Describe("truncate", func() {
	It("leaves short strings untouched", func() {
		Expect(truncate("hello", 10)).To(Equal("hello"))
	})

	It("truncates long strings and appends an ellipsis", func() {
		s := truncate("0123456789", 4)
		Expect(s).To(Equal("0123…"))
	})
})

var _ = Describe("managedTools and pipPackages", func() {
	It("has a pip package for every managed tool", func() {
		for _, tool := range managedTools {
			_, ok := pipPackages[tool]
			Expect(ok).To(BeTrue(), "missing pip package for %s", tool)
		}
	})

	It("never includes tidal, which is not pip-managed", func() {
		Expect(managedTools).ToNot(ContainElement(model.DownloadToolTidal))
		_, ok := pipPackages[model.DownloadToolTidal]
		Expect(ok).To(BeFalse())
	})
})
