package model

import "time"

type DownloadTool string

const (
	DownloadToolYtDlp      DownloadTool = "yt-dlp"
	DownloadToolScdl       DownloadTool = "scdl"
	DownloadToolSpotdl     DownloadTool = "spotdl"
	DownloadToolBandcampDl DownloadTool = "bandcamp-dl"
	DownloadToolTidal      DownloadTool = "tidal"
)

type DownloadStatus string

const (
	DownloadStatusQueued      DownloadStatus = "queued"
	DownloadStatusDownloading DownloadStatus = "downloading"
	DownloadStatusCompleted   DownloadStatus = "completed"
	DownloadStatusError       DownloadStatus = "error"
	DownloadStatusCanceled    DownloadStatus = "canceled"
)

// Download is a single job in the downloader queue, submitted either as a raw URL for an
// external tool (yt-dlp/scdl/spotdl/bandcamp-dl) or as a Tidal track/album id.
type Download struct {
	ID            string         `structs:"id" json:"id"`
	Tool          DownloadTool   `structs:"tool" json:"tool"`
	SourceURL     string         `structs:"source_url" json:"sourceUrl,omitempty"`
	TidalID       string         `structs:"tidal_id" json:"tidalId,omitempty"`
	TidalKind     string         `structs:"tidal_kind" json:"tidalKind,omitempty"`
	Status        DownloadStatus `structs:"status" json:"status"`
	Progress      float64        `structs:"progress" json:"progress"`
	StatusMessage string         `structs:"status_message" json:"statusMessage,omitempty"`
	Error         string         `structs:"error" json:"error,omitempty"`
	LibraryID     int            `structs:"library_id" json:"libraryId"`
	TargetPath    string         `structs:"target_path" json:"targetPath,omitempty"`
	RequestedBy   string         `structs:"requested_by" json:"requestedBy"`
	Attempts      int            `structs:"attempts" json:"attempts"`
	CreatedAt     time.Time      `structs:"created_at" json:"createdAt"`
	UpdatedAt     time.Time      `structs:"updated_at" json:"updatedAt"`
	StartedAt     *time.Time     `structs:"started_at" json:"startedAt,omitempty"`
	CompletedAt   *time.Time     `structs:"completed_at" json:"completedAt,omitempty"`
}

type Downloads []Download

// DownloadRepository is a plain typed repository (not a generic rest.Repository): the queue
// dashboard needs status-based filtering and job-lifecycle transitions rather than generic CRUD.
type DownloadRepository interface {
	Enqueue(d *Download) error
	Get(id string) (*Download, error)
	GetAll(options ...QueryOptions) (Downloads, error)
	DequeueBatch(n int) (Downloads, error)
	UpdateProgress(id string, progress float64, statusMessage string) error
	MarkStarted(id string) error
	MarkCompleted(id string, targetPath string) error
	MarkFailed(id string, errMsg string) error
	MarkCanceled(id string) error
	CountByStatus(status DownloadStatus) (int64, error)
}
