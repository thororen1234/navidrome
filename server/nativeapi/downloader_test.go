package nativeapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/conf/configtest"
	"github.com/navidrome/navidrome/consts"
	"github.com/navidrome/navidrome/core/auth"
	"github.com/navidrome/navidrome/core/downloader"
	"github.com/navidrome/navidrome/core/downloader/toolmgr"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/server"
	"github.com/navidrome/navidrome/server/events"
	"github.com/navidrome/navidrome/tests"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type fakeToolManager struct {
	status   []toolmgr.ToolStatus
	runErr   error
	runCalls []struct {
		Tool   model.DownloadTool
		Action toolmgr.Action
	}
}

func (f *fakeToolManager) Status(context.Context) []toolmgr.ToolStatus { return f.status }

func (f *fakeToolManager) Run(_ context.Context, tool model.DownloadTool, action toolmgr.Action) error {
	f.runCalls = append(f.runCalls, struct {
		Tool   model.DownloadTool
		Action toolmgr.Action
	}{tool, action})
	return f.runErr
}

type recordingBroker struct {
	http.Handler
	mu     sync.Mutex
	events []events.Event
}

func (b *recordingBroker) SendMessage(_ context.Context, event events.Event) { b.record(event) }
func (b *recordingBroker) SendBroadcastMessage(_ context.Context, event events.Event) {
	b.record(event)
}
func (b *recordingBroker) record(event events.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, event)
}
func (b *recordingBroker) getEvents() []events.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]events.Event(nil), b.events...)
}

var _ events.Broker = (*recordingBroker)(nil)

var _ = Describe("Downloader API", func() {
	var ds *tests.MockDataStore
	var router http.Handler
	var toolMgr *fakeToolManager
	var broker *recordingBroker
	var adminUser, regularUser model.User
	var adminToken, regularToken string

	doRequest := func(method, path string, body any, token string) *httptest.ResponseRecorder {
		var reqBody *bytes.Buffer
		if body != nil {
			b, _ := json.Marshal(body)
			reqBody = bytes.NewBuffer(b)
		} else {
			reqBody = bytes.NewBuffer(nil)
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
		conf.Server.Downloader.Enabled = true
		ds = &tests.MockDataStore{MockedLibrary: &tests.MockLibraryRepo{
			Data: map[int]model.Library{1: {ID: 1, Name: "Music", Path: "/music"}},
		}}
		toolMgr = &fakeToolManager{status: []toolmgr.ToolStatus{{Tool: model.DownloadToolYtDlp, Installed: true, Version: "2024.01.01"}}}
		broker = &recordingBroker{}
		auth.Init(ds)

		nativeRouter := New(ds, nil, nil, nil, tests.NewMockLibraryService(), tests.NewMockUserService(), nil, nil, nil,
			downloader.NewService(ds, downloader.NewRegistry()), toolMgr, broker, nil)
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

	Context("when the downloader is disabled", func() {
		BeforeEach(func() { conf.Server.Downloader.Enabled = false })

		It("404s the queue and tools endpoints", func() {
			Expect(doRequest("GET", "/download", nil, adminToken).Code).To(Equal(http.StatusNotFound))
			Expect(doRequest("GET", "/download/tools", nil, adminToken).Code).To(Equal(http.StatusNotFound))
		})
	})

	It("rejects non-admin users", func() {
		Expect(doRequest("GET", "/download", nil, regularToken).Code).To(Equal(http.StatusForbidden))
	})

	It("rejects a submission missing required fields", func() {
		w := doRequest("POST", "/download", map[string]any{"tool": "yt-dlp"}, adminToken)
		Expect(w.Code).To(Equal(http.StatusBadRequest))
	})

	It("submits, lists, and cancels a job", func() {
		w := doRequest("POST", "/download", map[string]any{
			"tool": "yt-dlp", "sourceUrl": "https://example.com/video", "libraryId": 1,
		}, adminToken)
		Expect(w.Code).To(Equal(http.StatusCreated))
		var created model.Download
		Expect(json.Unmarshal(w.Body.Bytes(), &created)).To(Succeed())
		Expect(created.ID).ToNot(BeEmpty())
		Expect(created.Status).To(Equal(model.DownloadStatusQueued))

		w = doRequest("GET", "/download", nil, adminToken)
		Expect(w.Code).To(Equal(http.StatusOK))
		var list model.Downloads
		Expect(json.Unmarshal(w.Body.Bytes(), &list)).To(Succeed())
		Expect(list).To(HaveLen(1))

		w = doRequest("DELETE", "/download/"+created.ID, nil, adminToken)
		Expect(w.Code).To(Equal(http.StatusNoContent))

		got, err := ds.Download(context.Background()).Get(created.ID)
		Expect(err).ToNot(HaveOccurred())
		Expect(got.Status).To(Equal(model.DownloadStatusCanceled))
	})

	It("rejects canceling a job that is not queued", func() {
		d := &model.Download{Tool: model.DownloadToolYtDlp, SourceURL: "https://example.com/x", LibraryID: 1}
		Expect(ds.Download(context.Background()).Enqueue(d)).To(Succeed())
		Expect(ds.Download(context.Background()).MarkCompleted(d.ID, "/music/x.mp3")).To(Succeed())

		w := doRequest("DELETE", "/download/"+d.ID, nil, adminToken)
		Expect(w.Code).To(Equal(http.StatusBadRequest))
	})

	It("reports tool status from the tool manager", func() {
		w := doRequest("GET", "/download/tools", nil, adminToken)
		Expect(w.Code).To(Equal(http.StatusOK))
		var status []toolmgr.ToolStatus
		Expect(json.Unmarshal(w.Body.Bytes(), &status)).To(Succeed())
		Expect(status).To(HaveLen(1))
		Expect(status[0].Tool).To(Equal(model.DownloadToolYtDlp))
		Expect(status[0].Installed).To(BeTrue())
	})

	It("runs a tool action and broadcasts before/after events", func() {
		w := doRequest("POST", "/download/tools/yt-dlp/upgrade", nil, adminToken)
		Expect(w.Code).To(Equal(http.StatusOK))
		Expect(toolMgr.runCalls).To(HaveLen(1))
		Expect(toolMgr.runCalls[0].Tool).To(Equal(model.DownloadToolYtDlp))
		Expect(toolMgr.runCalls[0].Action).To(Equal(toolmgr.ActionUpgrade))

		evts := broker.getEvents()
		Expect(evts).To(HaveLen(2))
		first := evts[0].(*events.ToolInstallStatus)
		Expect(first.Running).To(BeTrue())
		second := evts[1].(*events.ToolInstallStatus)
		Expect(second.Running).To(BeFalse())
		Expect(second.Error).To(BeEmpty())
	})

	It("rejects an unknown tool action", func() {
		w := doRequest("POST", "/download/tools/yt-dlp/bogus", nil, adminToken)
		Expect(w.Code).To(Equal(http.StatusBadRequest))
	})
})
