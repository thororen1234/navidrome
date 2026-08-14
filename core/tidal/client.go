// Package tidal is an HTTP client for TidalSubsonic, a separately-deployed sidecar (not part of
// this repo) that exposes a Subsonic-API-compatible bridge to Tidal. Navidrome
// never talks to Tidal directly: it authenticates to TidalSubsonic with one shared service
// account (conf.Server.Tidal.Username/Password) using the standard Subsonic token scheme, and
// every Navidrome user's browse/stream/download requests go through that single account.
package tidal

import (
	"context"
	"crypto/md5" //nolint:gosec // Subsonic's auth scheme mandates MD5; this is protocol compliance, not a security choice
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/log"
)

const (
	apiVersion = "1.16.1"
	clientName = "navidrome"
)

var ErrNotConfigured = errors.New("tidal: not configured (set Tidal.BaseURL/Username/Password)")

type Artist struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Albums []Album `json:"album"`
}

type Track struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Artist   string `json:"artist"`
	ArtistID string `json:"artistId"`
	Album    string `json:"album"`
	AlbumID  string `json:"albumId"`
	Duration int    `json:"duration"`
	CoverArt string `json:"coverArt"`
}

type Album struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Artist    string  `json:"artist"`
	ArtistID  string  `json:"artistId"`
	CoverArt  string  `json:"coverArt"`
	SongCount int     `json:"songCount"`
	Songs     []Track `json:"song"`
}

type SearchResult struct {
	Artists []Artist `json:"artist"`
	Albums  []Album  `json:"album"`
	Songs   []Track  `json:"song"`
}

// Client wraps TidalSubsonic's Subsonic-compatible REST API. Stream and DownloadTo are declared
// separately (stream.go) since they proxy raw bytes rather than JSON.
type Client interface {
	Search(ctx context.Context, query string) (SearchResult, error)
	GetArtist(ctx context.Context, id string) (Artist, error)
	GetAlbum(ctx context.Context, id string) (Album, error)
	// Stream returns the upstream response body/headers/status for a native API handler to copy
	// through to the browser, forwarding Range so in-browser seeking works. The caller must
	// close the returned body.
	Stream(ctx context.Context, id string, rangeHeader string) (body io.ReadCloser, header http.Header, statusCode int, err error)
	// CoverArt proxies TidalSubsonic's getCoverArt endpoint the same way Stream proxies audio.
	CoverArt(ctx context.Context, id string, size string) (body io.ReadCloser, header http.Header, statusCode int, err error)
	// DownloadTo streams the raw response for id (a single audio file for a track, a zip archive
	// for an album - TidalSubsonic always zips multi-track downloads) into w, returning the
	// upstream-suggested filename when one is present.
	DownloadTo(ctx context.Context, tidalID, kind string, w io.Writer) (filename string, err error)
}

type client struct {
	baseURL      string
	username     string
	password     string
	httpClient   httpDoer
	streamClient httpDoer
}

type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

func New() Client {
	return &client{
		baseURL:      strings.TrimSuffix(conf.Server.Tidal.BaseURL, "/"),
		username:     conf.Server.Tidal.Username,
		password:     conf.Server.Tidal.Password,
		httpClient:   &http.Client{Timeout: conf.Server.Tidal.Timeout},
		streamClient: &http.Client{Timeout: conf.Server.Tidal.StreamTimeout},
	}
}

func (c *client) configured() bool {
	return c.baseURL != "" && c.username != ""
}

// authParams returns a fresh Subsonic auth token set (t/s) plus the standard u/v/c/f params.
// A new salt is generated per call, as the Subsonic scheme requires.
func (c *client) authParams() url.Values {
	salt := randomSalt()
	v := url.Values{}
	v.Set("u", c.username)
	v.Set("t", md5Hex(c.password+salt))
	v.Set("s", salt)
	v.Set("v", apiVersion)
	v.Set("c", clientName)
	v.Set("f", "json")
	return v
}

func randomSalt() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func md5Hex(s string) string { //nolint:gosec // Subsonic's auth scheme mandates MD5
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

type subsonicErrorPayload struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// subsonicBase is embedded in every JSON response struct; its promoted subsonicStatus method is
// how get() checks the Subsonic-API-level status without a two-step decode.
type subsonicBase struct {
	Status string                `json:"status"`
	Error  *subsonicErrorPayload `json:"error,omitempty"`
}

func (b subsonicBase) subsonicStatus() (string, *subsonicErrorPayload) { return b.Status, b.Error }

type subsonicResult interface {
	subsonicStatus() (string, *subsonicErrorPayload)
}

type envelope struct {
	SubsonicResponse json.RawMessage `json:"subsonic-response"`
}

// get calls a Subsonic-style GET endpoint and decodes its "subsonic-response" payload into out,
// which must embed subsonicBase.
func (c *client) get(ctx context.Context, method string, extra url.Values, out subsonicResult) error {
	if !c.configured() {
		return ErrNotConfigured
	}
	params := c.authParams()
	for k, vals := range extra {
		for _, v := range vals {
			params.Add(k, v)
		}
	}
	reqURL := c.baseURL + "/rest/" + method + "?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return err
	}
	log.Trace(ctx, "Tidal: sending request", "method", method)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("tidal %s: %w", method, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("tidal %s: unexpected status %d", method, resp.StatusCode)
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return fmt.Errorf("tidal %s: decoding response: %w", method, err)
	}
	if err := json.Unmarshal(env.SubsonicResponse, out); err != nil {
		return fmt.Errorf("tidal %s: decoding payload: %w", method, err)
	}
	if status, sErr := out.subsonicStatus(); status != "ok" {
		msg := "unknown error"
		if sErr != nil {
			msg = sErr.Message
		}
		return fmt.Errorf("tidal %s: %s", method, msg)
	}
	return nil
}

type searchResponse struct {
	subsonicBase
	SearchResult3 SearchResult `json:"searchResult3"`
}

func (c *client) Search(ctx context.Context, query string) (SearchResult, error) {
	var resp searchResponse
	extra := url.Values{"query": {query}, "artistCount": {"20"}, "albumCount": {"20"}, "songCount": {"20"}}
	if err := c.get(ctx, "search3", extra, &resp); err != nil {
		return SearchResult{}, err
	}
	return resp.SearchResult3, nil
}

type artistResponse struct {
	subsonicBase
	Artist Artist `json:"artist"`
}

func (c *client) GetArtist(ctx context.Context, id string) (Artist, error) {
	var resp artistResponse
	if err := c.get(ctx, "getArtist", url.Values{"id": {id}}, &resp); err != nil {
		return Artist{}, err
	}
	return resp.Artist, nil
}

type albumResponse struct {
	subsonicBase
	Album Album `json:"album"`
}

func (c *client) GetAlbum(ctx context.Context, id string) (Album, error) {
	var resp albumResponse
	if err := c.get(ctx, "getAlbum", url.Values{"id": {id}}, &resp); err != nil {
		return Album{}, err
	}
	return resp.Album, nil
}

var _ Client = (*client)(nil)
