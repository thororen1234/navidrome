package nativeapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/downloader"
	"github.com/navidrome/navidrome/core/downloader/toolmgr"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
	"github.com/navidrome/navidrome/server/events"
)

// addDownloaderRoute wires the Downloader tab's queue management and tool install/upgrade/repair
// endpoints. The whole group is admin-only (see native_api.go's routes()) and 404s unless
// conf.Server.Downloader.Enabled.
func (api *Router) addDownloaderRoute(r chi.Router) {
	r.Route("/download", func(r chi.Router) {
		r.Use(downloaderEnabledMiddleware)
		r.Get("/", api.listDownloads)
		r.Post("/", api.submitDownload)
		r.Delete("/{id}", api.cancelDownload)
		r.Route("/tools", func(r chi.Router) {
			r.Get("/", api.listTools)
			r.Post("/{tool}/{action}", api.runToolAction)
		})
	})
}

func downloaderEnabledMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !conf.Server.Downloader.Enabled {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (api *Router) listDownloads(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	items, err := api.ds.Download(ctx).GetAll(model.QueryOptions{Sort: "created_at", Order: "desc"})
	if err != nil {
		log.Error(ctx, "Error listing downloads", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

type submitDownloadRequest struct {
	Tool      string `json:"tool"`
	SourceURL string `json:"sourceUrl"`
	TidalID   string `json:"tidalId"`
	TidalKind string `json:"tidalKind"`
	LibraryID int    `json:"libraryId"`
}

func (api *Router) submitDownload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req submitDownloadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	user, _ := request.UserFrom(ctx)
	d, err := api.downloader.Submit(ctx, downloader.SubmitRequest{
		Tool:      model.DownloadTool(req.Tool),
		SourceURL: req.SourceURL,
		TidalID:   req.TidalID,
		TidalKind: req.TidalKind,
		LibraryID: req.LibraryID,
		UserID:    user.ID,
	})
	if err != nil {
		if errors.Is(err, downloader.ErrInvalidRequest) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		log.Error(ctx, "Error submitting download", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, d)
}

func (api *Router) cancelDownload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	err := api.downloader.Cancel(ctx, id)
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, model.ErrNotFound):
		http.Error(w, "download not found", http.StatusNotFound)
	case errors.Is(err, downloader.ErrInvalidRequest):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		log.Error(ctx, "Error canceling download", "id", id, err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

func (api *Router) listTools(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, api.toolMgr.Status(r.Context()))
}

var validToolActions = map[string]toolmgr.Action{
	"install": toolmgr.ActionInstall,
	"upgrade": toolmgr.ActionUpgrade,
	"repair":  toolmgr.ActionRepair,
}

// runToolAction runs a pip/pipx install/upgrade/repair synchronously (these complete in a few
// seconds) and broadcasts before/after ToolInstallStatus events so the UI can show a spinner and
// the result without polling.
func (api *Router) runToolAction(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	toolName := chi.URLParam(r, "tool")
	actionName := chi.URLParam(r, "action")
	action, ok := validToolActions[actionName]
	if !ok {
		http.Error(w, "unknown action", http.StatusBadRequest)
		return
	}

	api.broker.SendBroadcastMessage(ctx, &events.ToolInstallStatus{Tool: toolName, Action: actionName, Running: true})
	runErr := api.toolMgr.Run(ctx, model.DownloadTool(toolName), action)
	result := &events.ToolInstallStatus{Tool: toolName, Action: actionName, Running: false}
	if runErr != nil {
		result.Error = runErr.Error()
	}
	api.broker.SendBroadcastMessage(ctx, result)

	if runErr != nil {
		log.Error(ctx, "Error running downloader tool action", "tool", toolName, "action", actionName, runErr)
		http.Error(w, runErr.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
