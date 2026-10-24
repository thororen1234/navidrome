package tidal

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("streamCache", func() {
	var dir string
	var fetches atomic.Int32

	newCache := func(contentType, body string, fetchErr error) *streamCache {
		return &streamCache{
			dir:     dir,
			timeout: time.Minute,
			fetch: func(_ context.Context, _ string, w io.Writer) (string, error) {
				fetches.Add(1)
				if fetchErr != nil {
					return "", fetchErr
				}
				_, err := io.Copy(w, strings.NewReader(body))
				return contentType, err
			},
			remux: func(_ context.Context, in, out string) error {
				data, err := os.ReadFile(in)
				if err != nil {
					return err
				}
				return os.WriteFile(out, append([]byte("remuxed:"), data...), 0600)
			},
		}
	}

	BeforeEach(func() {
		dir = GinkgoT().TempDir()
		fetches.Store(0)
	})

	It("downloads once and serves later requests from disk", func() {
		c := newCache("audio/flac", "fLaC-audio", nil)

		t1, err := c.Get(context.Background(), "123")
		Expect(err).ToNot(HaveOccurred())
		Expect(t1.Path).To(Equal(filepath.Join(dir, "123.flac")))
		Expect(t1.ContentType).To(Equal("audio/flac"))

		t2, err := c.Get(context.Background(), "123")
		Expect(err).ToNot(HaveOccurred())
		Expect(t2).To(Equal(t1))
		Expect(fetches.Load()).To(Equal(int32(1)))
	})

	It("shares one download between concurrent requests", func() {
		c := newCache("audio/flac", "fLaC-audio", nil)
		var wg sync.WaitGroup
		for range 5 {
			wg.Go(func() {
				defer GinkgoRecover()
				_, err := c.Get(context.Background(), "123")
				Expect(err).ToNot(HaveOccurred())
			})
		}
		wg.Wait()
		Expect(fetches.Load()).To(Equal(int32(1)))
	})

	It("remuxes a fragmented MP4 stream into a regular .m4a", func() {
		frag := string(append(box("ftyp", []byte("iso6")), box("moof")...))
		c := newCache("audio/mp4", frag, nil)

		t, err := c.Get(context.Background(), "123")
		Expect(err).ToNot(HaveOccurred())
		Expect(t.Path).To(Equal(filepath.Join(dir, "123.m4a")))
		Expect(t.ContentType).To(Equal("audio/mp4"))
		data, _ := os.ReadFile(t.Path)
		Expect(string(data)).To(HavePrefix("remuxed:"))
	})

	It("sniffs the format when the upstream Content-Type is generic", func() {
		c := newCache("application/octet-stream", "fLaC-audio", nil)
		t, err := c.Get(context.Background(), "123")
		Expect(err).ToNot(HaveOccurred())
		Expect(t.ContentType).To(Equal("audio/flac"))
	})

	It("leaves nothing behind when the download fails", func() {
		c := newCache("", "", errors.New("boom"))
		_, err := c.Get(context.Background(), "123")
		Expect(err).To(MatchError("boom"))
		entries, _ := os.ReadDir(dir)
		Expect(entries).To(BeEmpty())
	})

	It("never lets an id escape the cache dir", func() {
		c := newCache("audio/flac", "fLaC", nil)
		t, err := c.Get(context.Background(), "../../etc/passwd")
		Expect(err).ToNot(HaveOccurred())
		Expect(filepath.Dir(t.Path)).To(Equal(dir))
	})

	It("evicts least-recently-used tracks beyond maxBytes, keeping the newest", func() {
		c := newCache("audio/flac", "fLaC-0123456789", nil) // 15 bytes per track
		c.maxBytes = 40
		old := time.Now().Add(-time.Hour)
		for i, id := range []string{"a", "b", "c"} {
			t, err := c.Get(context.Background(), id)
			Expect(err).ToNot(HaveOccurred())
			ts := old.Add(time.Duration(i) * time.Minute)
			Expect(os.Chtimes(t.Path, ts, ts)).To(Succeed())
		}
		c.prune(filepath.Join(dir, "c.flac"))

		Expect(filepath.Join(dir, "a.flac")).ToNot(BeAnExistingFile())
		Expect(filepath.Join(dir, "b.flac")).To(BeAnExistingFile())
		Expect(filepath.Join(dir, "c.flac")).To(BeAnExistingFile())
	})
})

func box(boxType string, payload ...[]byte) []byte {
	var body []byte
	for _, p := range payload {
		body = append(body, p...)
	}
	size := 8 + len(body)
	out := []byte{byte(size >> 24), byte(size >> 16), byte(size >> 8), byte(size)}
	out = append(out, boxType...)
	return append(out, body...)
}
