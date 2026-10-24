package tidal

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
)

func (c *client) OpenStream(ctx context.Context, id string) (CachedTrack, error) {
	if !c.configured() {
		return CachedTrack{}, ErrNotConfigured
	}
	return c.streams.Get(ctx, id)
}

// fetchStream copies the whole upstream stream for id into w, for streamCache to fill from.
func (c *client) fetchStream(ctx context.Context, id string, w io.Writer) (string, error) {
	body, header, _, err := c.Stream(ctx, id, "")
	if err != nil {
		return "", err
	}
	defer body.Close()
	if _, err := io.Copy(w, body); err != nil {
		return "", fmt.Errorf("tidal stream %s: %w", id, err)
	}
	return header.Get("Content-Type"), nil
}

// Stream returns the raw upstream stream response, forwarding rangeHeader when set. The caller
// must close the returned body.
func (c *client) Stream(ctx context.Context, id string, rangeHeader string) (io.ReadCloser, http.Header, int, error) {
	if !c.configured() {
		return nil, nil, 0, ErrNotConfigured
	}
	params := c.authParams()
	params.Set("id", id)
	reqURL := c.baseURL + "/rest/stream?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, nil, 0, err
	}
	if rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}
	resp, err := c.streamClient.Do(req)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("tidal stream %s: %w", id, err)
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		return nil, nil, 0, fmt.Errorf("tidal stream %s: unexpected status %d", id, resp.StatusCode)
	}
	return resp.Body, resp.Header, resp.StatusCode, nil
}

// CoverArt proxies TidalSubsonic's getCoverArt endpoint. size, when non-empty, is forwarded
// as-is so the caller can request a scaled-down image (per the Subsonic API's "size" param).
func (c *client) CoverArt(ctx context.Context, id string, size string) (io.ReadCloser, http.Header, int, error) {
	if !c.configured() {
		return nil, nil, 0, ErrNotConfigured
	}
	params := c.authParams()
	params.Set("id", id)
	if size != "" {
		params.Set("size", size)
	}
	reqURL := c.baseURL + "/rest/getCoverArt?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, nil, 0, err
	}
	resp, err := c.streamClient.Do(req)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("tidal coverArt %s: %w", id, err)
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound {
			return nil, nil, 0, fmt.Errorf("tidal coverArt %s: %w", id, ErrNotFound)
		}
		return nil, nil, 0, fmt.Errorf("tidal coverArt %s: unexpected status %d", id, resp.StatusCode)
	}
	return resp.Body, resp.Header, resp.StatusCode, nil
}

func (c *client) DownloadTo(ctx context.Context, tidalID, kind string, w io.Writer, onSize func(int64)) (string, error) {
	if !c.configured() {
		return "", ErrNotConfigured
	}
	params := c.authParams()
	params.Set("id", tidalID)
	reqURL := c.baseURL + "/rest/download?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.streamClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("tidal download %s: %w", tidalID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("tidal download %s: unexpected status %d", tidalID, resp.StatusCode)
	}
	filename := filenameFromResponse(resp, tidalID, kind)
	if onSize != nil {
		onSize(resp.ContentLength)
	}
	if _, err := io.Copy(w, resp.Body); err != nil {
		return "", fmt.Errorf("tidal download %s: %w", tidalID, err)
	}
	return filename, nil
}

// filenameFromResponse prefers the upstream Content-Disposition filename; falling back to a
// name derived from the id/kind keeps DownloadTo usable even if TidalSubsonic omits the header.
func filenameFromResponse(resp *http.Response, id, kind string) string {
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		if _, params, err := mime.ParseMediaType(cd); err == nil && params["filename"] != "" {
			return params["filename"]
		}
	}
	if kind == "album" {
		return id + ".zip"
	}
	ext := ".flac"
	mediaType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if e, ok := audioExtByType[mediaType]; ok {
		ext = e
	}
	return id + ext
}
