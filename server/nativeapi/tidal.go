package nativeapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/downloader"
	"github.com/navidrome/navidrome/core/tidal"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
)

// addTidalRoute wires the Tidal library tab: browse/search/stream are available to any
// authenticated user, while "download to server" is admin-only (it writes to the filesystem via
// the same queue/worker as the Downloader tab) and separately gated by Tidal.AllowDownload.
func (api *Router) addTidalRoute(r chi.Router) {
	r.Route("/tidal", func(r chi.Router) {
		r.Use(tidalEnabledMiddleware)
		r.Get("/search", api.tidalSearch)
		r.Get("/artist/{id}", api.tidalArtist)
		r.Get("/album/{id}", api.tidalAlbum)
		r.Get("/stream/{id}", api.tidalStream)
		r.Get("/coverArt/{id}", api.tidalCoverArt)
		r.With(adminOnlyMiddleware).Post("/download", api.tidalDownload)
	})
}

func tidalEnabledMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !conf.Server.Tidal.Enabled {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (api *Router) tidalSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query().Get("q")
	if q == "" {
		http.Error(w, "missing query parameter q", http.StatusBadRequest)
		return
	}
	result, err := api.tidal.Search(ctx, q)
	if err != nil {
		log.Error(ctx, "Error searching Tidal", "query", q, err)
		http.Error(w, "error searching tidal", http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (api *Router) tidalArtist(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	artist, err := api.tidal.GetArtist(ctx, id)
	if err != nil {
		log.Error(ctx, "Error getting Tidal artist", "id", id, err)
		http.Error(w, "error fetching tidal artist", http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, artist)
}

func (api *Router) tidalAlbum(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	album, err := api.tidal.GetAlbum(ctx, id)
	if err != nil {
		log.Error(ctx, "Error getting Tidal album", "id", id, err)
		http.Error(w, "error fetching tidal album", http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, album)
}

// tidalStream serves a Tidal track from Navidrome's local stream cache (see tidal.streamCache):
// the first request downloads the whole track from TidalSubsonic, so the browser gets a regular
// file with a real duration, Content-Length and Range support instead of TidalSubsonic's
// unseekable fragmented stream. The browser never talks to TidalSubsonic directly.
func (api *Router) tidalStream(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	track, err := api.tidal.OpenStream(ctx, id)
	if err != nil {
		if ctx.Err() != nil {
			return // client went away while the track was downloading
		}
		log.Error(ctx, "Error streaming from Tidal", "id", id, err)
		http.Error(w, "error streaming from tidal", http.StatusBadGateway)
		return
	}
	f, err := os.Open(track.Path)
	if err != nil {
		log.Error(ctx, "Error opening cached Tidal stream", "id", id, "path", track.Path, err)
		http.Error(w, "error streaming from tidal", http.StatusInternalServerError)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		http.Error(w, "error streaming from tidal", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", track.ContentType)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeContent(w, r, "", info.ModTime(), f)
}

// tidalCoverArt proxies TidalSubsonic's getCoverArt response through Navidrome, same rationale
// as tidalStream: the browser never needs direct network access to TidalSubsonic. A missing
// image is a plain 404 (the UI falls back to a placeholder icon), not a logged error.
func (api *Router) tidalCoverArt(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	size := r.URL.Query().Get("size")
	body, header, status, err := api.tidal.CoverArt(ctx, id, size)
	if err != nil {
		if errors.Is(err, tidal.ErrNotFound) {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		log.Warn(ctx, "Error fetching cover art from Tidal", "id", id, err)
		http.Error(w, "error fetching tidal cover art", http.StatusBadGateway)
		return
	}
	defer body.Close()
	for _, h := range []string{"Content-Type", "Content-Length"} {
		if v := header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.WriteHeader(status)
	_, _ = io.Copy(w, body)
}

type tidalDownloadRequest struct {
	TidalID   string `json:"tidalId"`
	TidalKind string `json:"tidalKind"`
	LibraryID int    `json:"libraryId"`
}

// tidalDownload enqueues a "download to server" job into the same queue/worker the Downloader
// tab uses (Tool: tidal), rather than fetching anything itself.
func (api *Router) tidalDownload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !conf.Server.Tidal.AllowDownload {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	if !conf.Server.Downloader.Enabled {
		http.Error(w, "Downloader is disabled; enable it to download to the server", http.StatusServiceUnavailable)
		return
	}
	var req tidalDownloadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	user, _ := request.UserFrom(ctx)
	d, err := api.downloader.Submit(ctx, downloader.SubmitRequest{
		Tool:      model.DownloadToolTidal,
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
		log.Error(ctx, "Error submitting Tidal download", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, d)
}
