package downloader

import (
	"testing"

	"github.com/navidrome/navidrome/log"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestDownloader(t *testing.T) {
	log.SetLevel(log.LevelFatal)
	RegisterFailHandler(Fail)
	RunSpecs(t, "Downloader Suite")
}
