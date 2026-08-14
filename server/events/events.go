package events

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"time"
	"unicode"
)

type eventCtxKey string

const broadcastToAllKey eventCtxKey = "broadcastToAll"

// broadcastToAll is a context key that can be used to broadcast an event to all clients
func broadcastToAll(ctx context.Context) context.Context {
	return context.WithValue(ctx, broadcastToAllKey, true)
}

type Event interface {
	Name(Event) string
	Data(Event) string
}

type baseEvent struct{}

func (e *baseEvent) Name(evt Event) string {
	str := strings.TrimPrefix(reflect.TypeOf(evt).String(), "*events.")
	return str[:0] + string(unicode.ToLower(rune(str[0]))) + str[1:]
}

func (e *baseEvent) Data(evt Event) string {
	data, _ := json.Marshal(evt)
	return string(data)
}

type ScanStatus struct {
	baseEvent
	Scanning    bool          `json:"scanning"`
	Count       int64         `json:"count"`
	FolderCount int64         `json:"folderCount"`
	Error       string        `json:"error"`
	ScanType    string        `json:"scanType"`
	ElapsedTime time.Duration `json:"elapsedTime"`
}

type KeepAlive struct {
	baseEvent
	TS int64 `json:"ts"`
}

type ServerStart struct {
	baseEvent
	StartTime time.Time `json:"startTime"`
	Version   string    `json:"version"`
}

const Any = "*"

type RefreshResource struct {
	baseEvent
	resources map[string][]string
}

type NowPlayingCount struct {
	baseEvent
	Count int `json:"count"`
}

// DownloadStatus reports a downloader queue job's current state, broadcast whenever the worker
// updates progress, starts, completes, or fails a job.
type DownloadStatus struct {
	baseEvent
	ID            string  `json:"id"`
	Status        string  `json:"status"`
	Progress      float64 `json:"progress"`
	StatusMessage string  `json:"statusMessage,omitempty"`
	Error         string  `json:"error,omitempty"`
}

// ToolInstallStatus reports progress of a downloader tool's install/upgrade/repair action.
type ToolInstallStatus struct {
	baseEvent
	Tool    string `json:"tool"`
	Action  string `json:"action"`
	Running bool   `json:"running"`
	Output  string `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
}

func (rr *RefreshResource) With(resource string, ids ...string) *RefreshResource {
	if rr.resources == nil {
		rr.resources = make(map[string][]string)
	}
	if len(ids) == 0 {
		rr.resources[resource] = append(rr.resources[resource], Any)
	}
	rr.resources[resource] = append(rr.resources[resource], ids...)
	return rr
}

func (rr *RefreshResource) Data(evt Event) string {
	if rr.resources == nil {
		return `{"*":"*"}`
	}
	r := evt.(*RefreshResource)
	data, _ := json.Marshal(r.resources)
	return string(data)
}
