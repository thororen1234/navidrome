package downloader

import (
	"context"
	"os"
	"path/filepath"

	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/tests"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("sanitizeFilename", func() {
	It("keeps a normal filename as-is", func() {
		Expect(sanitizeFilename("My Song.mp3")).To(Equal("My Song.mp3"))
	})

	It("strips path separators and traversal", func() {
		Expect(sanitizeFilename("../../etc/passwd")).To(Equal("passwd"))
		Expect(sanitizeFilename("a/b\\c.mp3")).To(Equal("c.mp3"))
	})

	It("replaces filesystem-hostile characters", func() {
		Expect(sanitizeFilename(`weird:name?.mp3`)).To(Equal("weird_name_.mp3"))
	})

	It("falls back to a default name for empty or dot-only input", func() {
		Expect(sanitizeFilename("")).To(Equal("download"))
		Expect(sanitizeFilename(".")).To(Equal("download"))
		Expect(sanitizeFilename("..")).To(Equal("download"))
	})
})

var _ = Describe("resolveTargetDir", func() {
	It("joins the library path, Downloads, and the tool name", func() {
		ds := &tests.MockDataStore{MockedLibrary: &tests.MockLibraryRepo{
			Data: map[int]model.Library{1: {ID: 1, Path: filepath.FromSlash("/music")}},
		}}
		dir, err := resolveTargetDir(context.Background(), ds, &model.Download{LibraryID: 1, Tool: model.DownloadToolYtDlp})
		Expect(err).ToNot(HaveOccurred())
		Expect(dir).To(Equal(filepath.Join(filepath.FromSlash("/music"), "Downloads", "yt-dlp")))
	})

	It("errors when the library does not exist", func() {
		ds := &tests.MockDataStore{MockedLibrary: &tests.MockLibraryRepo{Data: map[int]model.Library{}}}
		_, err := resolveTargetDir(context.Background(), ds, &model.Download{LibraryID: 99})
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("moveJobOutput", func() {
	It("moves files from staging into the target dir and removes staging", func() {
		staging := GinkgoT().TempDir()
		target := filepath.Join(GinkgoT().TempDir(), "target")
		Expect(os.WriteFile(filepath.Join(staging, "track.mp3"), []byte("data"), 0644)).To(Succeed())
		sub := filepath.Join(staging, "extras")
		Expect(os.Mkdir(sub, 0755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(sub, "cover.jpg"), []byte("img"), 0644)).To(Succeed())

		moved, last, err := moveJobOutput(staging, target)
		Expect(err).ToNot(HaveOccurred())
		Expect(moved).To(Equal(2))
		Expect(last).ToNot(BeEmpty())
		Expect(filepath.Join(target, "track.mp3")).To(BeAnExistingFile())
		Expect(filepath.Join(target, "cover.jpg")).To(BeAnExistingFile())
		_, err = os.Stat(staging)
		Expect(os.IsNotExist(err)).To(BeTrue())
	})

	It("disambiguates same-named files instead of overwriting", func() {
		staging := GinkgoT().TempDir()
		target := GinkgoT().TempDir()
		Expect(os.WriteFile(filepath.Join(target, "cover.jpg"), []byte("existing"), 0644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(staging, "cover.jpg"), []byte("new"), 0644)).To(Succeed())

		moved, _, err := moveJobOutput(staging, target)
		Expect(err).ToNot(HaveOccurred())
		Expect(moved).To(Equal(1))
		Expect(filepath.Join(target, "cover.jpg")).To(BeAnExistingFile())
		Expect(filepath.Join(target, "cover (1).jpg")).To(BeAnExistingFile())
	})

	It("reports zero moved for an empty staging dir", func() {
		staging := GinkgoT().TempDir()
		target := GinkgoT().TempDir()
		moved, last, err := moveJobOutput(staging, target)
		Expect(err).ToNot(HaveOccurred())
		Expect(moved).To(Equal(0))
		Expect(last).To(BeEmpty())
	})
})
