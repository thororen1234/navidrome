package tidal

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
)

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
		return nil, nil, 0, fmt.Errorf("tidal coverArt %s: unexpected status %d", id, resp.StatusCode)
	}
	return resp.Body, resp.Header, resp.StatusCode, nil
}

func (c *client) DownloadTo(ctx context.Context, tidalID, kind string, w io.Writer) (string, error) {
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
	ext := ".flac"
	if kind == "album" {
		ext = ".zip"
	}
	return id + ext
}
