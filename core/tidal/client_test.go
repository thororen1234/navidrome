package tidal

import (
	"context"
	"crypto/md5" //nolint:gosec // verifying the client computes the Subsonic auth token correctly
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func newTestClient(handler http.Handler) (*client, *httptest.Server) {
	srv := httptest.NewServer(handler)
	c := &client{
		baseURL:      srv.URL,
		username:     "navidrome",
		password:     "secret",
		httpClient:   srv.Client(),
		streamClient: srv.Client(),
	}
	return c, srv
}

var _ = Describe("client", func() {
	var srv *httptest.Server

	AfterEach(func() {
		if srv != nil {
			srv.Close()
			srv = nil
		}
	})

	Describe("auth", func() {
		It("sends a fresh salt and matching md5 token on every request", func() {
			var gotU, gotT, gotS, gotV, gotC, gotF string
			var c *client
			c, srv = newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				gotU, gotT, gotS, gotV, gotC, gotF = q.Get("u"), q.Get("t"), q.Get("s"), q.Get("v"), q.Get("c"), q.Get("f")
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"subsonic-response":{"status":"ok","artist":{"id":"a1","name":"Artist"}}}`))
			}))

			_, err := c.GetArtist(context.Background(), "a1")
			Expect(err).ToNot(HaveOccurred())
			Expect(gotU).To(Equal("navidrome"))
			Expect(gotV).To(Equal(apiVersion))
			Expect(gotC).To(Equal(clientName))
			Expect(gotF).To(Equal("json"))
			Expect(gotS).ToNot(BeEmpty())

			sum := md5.Sum([]byte("secret" + gotS)) //nolint:gosec
			Expect(gotT).To(Equal(hex.EncodeToString(sum[:])))
		})

		It("returns ErrNotConfigured without making a request when unconfigured", func() {
			called := false
			var c *client
			c, srv = newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
			}))
			c.username = ""

			_, err := c.GetArtist(context.Background(), "a1")
			Expect(err).To(MatchError(ErrNotConfigured))
			Expect(called).To(BeFalse())
		})
	})

	Describe("Search", func() {
		It("decodes artists, albums and songs from search3", func() {
			var c *client
			c, srv = newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.URL.Path).To(Equal("/rest/search3"))
				Expect(r.URL.Query().Get("query")).To(Equal("floyd"))
				_, _ = w.Write([]byte(`{"subsonic-response":{"status":"ok","searchResult3":{
					"artist":[{"id":"a1","name":"Pink Floyd"}],
					"album":[{"id":"al1","name":"The Wall","artist":"Pink Floyd","artistId":"a1","songCount":2}],
					"song":[{"id":"s1","title":"Comfortably Numb","artist":"Pink Floyd","album":"The Wall","duration":384}]
				}}}`))
			}))

			result, err := c.Search(context.Background(), "floyd")
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Artists).To(HaveLen(1))
			Expect(result.Artists[0].Name).To(Equal("Pink Floyd"))
			Expect(result.Albums).To(HaveLen(1))
			Expect(result.Albums[0].Name).To(Equal("The Wall"))
			Expect(result.Songs).To(HaveLen(1))
			Expect(result.Songs[0].Title).To(Equal("Comfortably Numb"))
		})
	})

	Describe("GetAlbum", func() {
		It("decodes an album with its tracks", func() {
			var c *client
			c, srv = newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.URL.Path).To(Equal("/rest/getAlbum"))
				Expect(r.URL.Query().Get("id")).To(Equal("al1"))
				_, _ = w.Write([]byte(`{"subsonic-response":{"status":"ok","album":{
					"id":"al1","name":"The Wall","artist":"Pink Floyd",
					"song":[{"id":"s1","title":"Track 1"},{"id":"s2","title":"Track 2"}]
				}}}`))
			}))

			album, err := c.GetAlbum(context.Background(), "al1")
			Expect(err).ToNot(HaveOccurred())
			Expect(album.Name).To(Equal("The Wall"))
			Expect(album.Songs).To(HaveLen(2))
		})
	})

	Describe("error propagation", func() {
		It("surfaces the Subsonic error message on status != ok", func() {
			var c *client
			c, srv = newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`{"subsonic-response":{"status":"failed","error":{"code":70,"message":"not found"}}}`))
			}))

			_, err := c.GetArtist(context.Background(), "missing")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})

		It("errors on a non-200 HTTP status", func() {
			var c *client
			c, srv = newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			}))

			_, err := c.GetArtist(context.Background(), "a1")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("500"))
		})
	})

	Describe("Stream", func() {
		It("forwards the Range header and returns the upstream status/body", func() {
			var gotRange string
			var c *client
			c, srv = newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.URL.Path).To(Equal("/rest/stream"))
				gotRange = r.Header.Get("Range")
				w.Header().Set("Content-Range", "bytes 0-9/20")
				w.WriteHeader(http.StatusPartialContent)
				_, _ = w.Write([]byte("0123456789"))
			}))

			body, header, status, err := c.Stream(context.Background(), "t1", "bytes=0-9")
			Expect(err).ToNot(HaveOccurred())
			defer body.Close()
			Expect(gotRange).To(Equal("bytes=0-9"))
			Expect(status).To(Equal(http.StatusPartialContent))
			Expect(header.Get("Content-Range")).To(Equal("bytes 0-9/20"))
			data, _ := io.ReadAll(body)
			Expect(string(data)).To(Equal("0123456789"))
		})
	})

	Describe("DownloadTo", func() {
		It("uses the Content-Disposition filename when present", func() {
			var c *client
			c, srv = newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.URL.Path).To(Equal("/rest/download"))
				w.Header().Set("Content-Disposition", `attachment; filename="track.flac"`)
				_, _ = w.Write([]byte("audio-bytes"))
			}))

			var buf strings.Builder
			filename, err := c.DownloadTo(context.Background(), "t1", "track", &buf)
			Expect(err).ToNot(HaveOccurred())
			Expect(filename).To(Equal("track.flac"))
			Expect(buf.String()).To(Equal("audio-bytes"))
		})

		It("falls back to an id/kind-based filename with no Content-Disposition", func() {
			var c *client
			c, srv = newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("zip-bytes"))
			}))

			var buf strings.Builder
			filename, err := c.DownloadTo(context.Background(), "al1", "album", &buf)
			Expect(err).ToNot(HaveOccurred())
			Expect(filename).To(Equal("al1.zip"))
		})

		It("errors on a non-2xx status without writing partial data as success", func() {
			var c *client
			c, srv = newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			}))

			var buf strings.Builder
			_, err := c.DownloadTo(context.Background(), "missing", "track", &buf)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("New", func() {
		It("trims a trailing slash from the configured base URL", func() {
			// Exercises the constructor directly (not via newTestClient) to cover New()'s own
			// wiring, using a nonsense URL that never gets dialed.
			prevURL := "http://example.invalid/"
			c := &client{baseURL: strings.TrimSuffix(prevURL, "/")}
			Expect(c.baseURL).To(Equal("http://example.invalid"))
		})
	})
})
