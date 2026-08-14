package nativeapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/conf/configtest"
	"github.com/navidrome/navidrome/consts"
	"github.com/navidrome/navidrome/core/auth"
	"github.com/navidrome/navidrome/core/downloader"
	"github.com/navidrome/navidrome/core/tidal"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/server"
	"github.com/navidrome/navidrome/tests"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type fakeTidalClient struct {
	searchResult tidal.SearchResult
	artist       tidal.Artist
	album        tidal.Album
	streamBody   string
	streamHeader http.Header
	streamStatus int
	err          error
}

func (f *fakeTidalClient) Search(context.Context, string) (tidal.SearchResult, error) {
	return f.searchResult, f.err
}

func (f *fakeTidalClient) GetArtist(context.Context, string) (tidal.Artist, error) {
	return f.artist, f.err
}

func (f *fakeTidalClient) GetAlbum(context.Context, string) (tidal.Album, error) {
	return f.album, f.err
}

func (f *fakeTidalClient) Stream(context.Context, string, string) (io.ReadCloser, http.Header, int, error) {
	if f.err != nil {
		return nil, nil, 0, f.err
	}
	h := f.streamHeader
	if h == nil {
		h = http.Header{}
	}
	status := f.streamStatus
	if status == 0 {
		status = http.StatusOK
	}
	return io.NopCloser(strings.NewReader(f.streamBody)), h, status, nil
}

func (f *fakeTidalClient) DownloadTo(context.Context, string, string, io.Writer) (string, error) {
	return "track.flac", f.err
}

var _ tidal.Client = (*fakeTidalClient)(nil)

var _ = Describe("Tidal API", func() {
	var ds *tests.MockDataStore
	var router http.Handler
	var fakeTidal *fakeTidalClient
	var adminUser, regularUser model.User
	var adminToken, regularToken string

	doRequest := func(method, path string, body any, token string) *httptest.ResponseRecorder {
		var reqBody *strings.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			reqBody = strings.NewReader(string(b))
		} else {
			reqBody = strings.NewReader("")
		}
		req := httptest.NewRequest(method, path, reqBody)
		req.Header.Set(consts.UIAuthorizationHeader, "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	BeforeEach(func() {
		DeferCleanup(configtest.SetupConfig())
		conf.Server.EnableSharing = false
		conf.Server.Tidal.Enabled = true
		conf.Server.Tidal.AllowDownload = true
		conf.Server.Downloader.Enabled = true
		ds = &tests.MockDataStore{MockedLibrary: &tests.MockLibraryRepo{
			Data: map[int]model.Library{1: {ID: 1, Name: "Music", Path: "/music"}},
		}}
		fakeTidal = &fakeTidalClient{}
		auth.Init(ds)

		nativeRouter := New(ds, nil, nil, nil, tests.NewMockLibraryService(), tests.NewMockUserService(), nil, nil, nil,
			downloader.NewService(ds, downloader.NewRegistry()), nil, nil, fakeTidal)
		router = server.JWTVerifier(nativeRouter)

		adminUser = model.User{ID: "admin-1", UserName: "admin", Name: "Admin", IsAdmin: true, NewPassword: "adminpass"}
		regularUser = model.User{ID: "user-1", UserName: "regular", Name: "Regular", IsAdmin: false, NewPassword: "userpass"}
		Expect(ds.User(context.Background()).Put(&adminUser)).To(Succeed())
		Expect(ds.User(context.Background()).Put(&regularUser)).To(Succeed())
		var err error
		adminToken, err = auth.CreateToken(&adminUser)
		Expect(err).ToNot(HaveOccurred())
		regularToken, err = auth.CreateToken(&regularUser)
		Expect(err).ToNot(HaveOccurred())
	})

	Context("when Tidal is disabled", func() {
		BeforeEach(func() { conf.Server.Tidal.Enabled = false })

		It("404s browse and stream endpoints", func() {
			Expect(doRequest("GET", "/tidal/search?q=floyd", nil, regularToken).Code).To(Equal(http.StatusNotFound))
			Expect(doRequest("GET", "/tidal/stream/t1", nil, regularToken).Code).To(Equal(http.StatusNotFound))
		})
	})

	It("lets any authenticated user search", func() {
		fakeTidal.searchResult = tidal.SearchResult{Artists: []tidal.Artist{{ID: "a1", Name: "Pink Floyd"}}}
		w := doRequest("GET", "/tidal/search?q=floyd", nil, regularToken)
		Expect(w.Code).To(Equal(http.StatusOK))
		var result tidal.SearchResult
		Expect(json.Unmarshal(w.Body.Bytes(), &result)).To(Succeed())
		Expect(result.Artists).To(HaveLen(1))
	})

	It("requires a query parameter for search", func() {
		w := doRequest("GET", "/tidal/search", nil, regularToken)
		Expect(w.Code).To(Equal(http.StatusBadRequest))
	})

	It("lets any authenticated user fetch an artist", func() {
		fakeTidal.artist = tidal.Artist{ID: "a1", Name: "Pink Floyd"}
		w := doRequest("GET", "/tidal/artist/a1", nil, regularToken)
		Expect(w.Code).To(Equal(http.StatusOK))
		var artist tidal.Artist
		Expect(json.Unmarshal(w.Body.Bytes(), &artist)).To(Succeed())
		Expect(artist.Name).To(Equal("Pink Floyd"))
	})

	It("lets any authenticated user fetch an album", func() {
		fakeTidal.album = tidal.Album{ID: "al1", Name: "The Wall"}
		w := doRequest("GET", "/tidal/album/al1", nil, regularToken)
		Expect(w.Code).To(Equal(http.StatusOK))
		var album tidal.Album
		Expect(json.Unmarshal(w.Body.Bytes(), &album)).To(Succeed())
		Expect(album.Name).To(Equal("The Wall"))
	})

	It("proxies the stream response body and headers", func() {
		fakeTidal.streamBody = "audio-bytes"
		fakeTidal.streamHeader = http.Header{"Content-Type": []string{"audio/flac"}}
		fakeTidal.streamStatus = http.StatusPartialContent
		w := doRequest("GET", "/tidal/stream/t1", nil, regularToken)
		Expect(w.Code).To(Equal(http.StatusPartialContent))
		Expect(w.Header().Get("Content-Type")).To(Equal("audio/flac"))
		Expect(w.Body.String()).To(Equal("audio-bytes"))
	})

	It("rejects download-to-server from a non-admin user", func() {
		w := doRequest("POST", "/tidal/download", map[string]any{
			"tidalId": "t1", "tidalKind": "track", "libraryId": 1,
		}, regularToken)
		Expect(w.Code).To(Equal(http.StatusForbidden))
	})

	It("submits a download-to-server job for an admin", func() {
		w := doRequest("POST", "/tidal/download", map[string]any{
			"tidalId": "t1", "tidalKind": "track", "libraryId": 1,
		}, adminToken)
		Expect(w.Code).To(Equal(http.StatusCreated))
		var d model.Download
		Expect(json.Unmarshal(w.Body.Bytes(), &d)).To(Succeed())
		Expect(d.Tool).To(Equal(model.DownloadToolTidal))
		Expect(d.TidalID).To(Equal("t1"))
		Expect(d.Status).To(Equal(model.DownloadStatusQueued))
	})

	It("404s download-to-server when AllowDownload is off", func() {
		conf.Server.Tidal.AllowDownload = false
		w := doRequest("POST", "/tidal/download", map[string]any{
			"tidalId": "t1", "tidalKind": "track", "libraryId": 1,
		}, adminToken)
		Expect(w.Code).To(Equal(http.StatusNotFound))
	})

	It("rejects download-to-server when the Downloader worker is disabled", func() {
		conf.Server.Downloader.Enabled = false
		w := doRequest("POST", "/tidal/download", map[string]any{
			"tidalId": "t1", "tidalKind": "track", "libraryId": 1,
		}, adminToken)
		Expect(w.Code).To(Equal(http.StatusServiceUnavailable))
	})
})
