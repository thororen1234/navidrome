package model

import (
	"context"
	"time"
)

type DownloadTool string

const (
	DownloadToolYtDlp      DownloadTool = "yt-dlp"
	DownloadToolScdl       DownloadTool = "scdl"
	DownloadToolSpotdl     DownloadTool = "spotdl"
	DownloadToolBandcampDl DownloadTool = "bandcamp-dl"
	DownloadToolKhinsider  DownloadTool = "khinsider"
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
// external tool (yt-dlp/scdl/spotdl/bandcamp-dl/khinsider) or as a Tidal track/album id.
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
//
// Lifecycle transitions that race between the API and the worker (claiming, canceling a queued
// job, retrying) are conditional on the current status and report whether they applied, so two
// writers can never both "win" the same job.
type DownloadRepository interface {
	Enqueue(ctx context.Context, d *Download) error
	Get(ctx context.Context, id string) (*Download, error)
	GetAll(ctx context.Context, options ...QueryOptions) (Downloads, error)
	// ClaimNext atomically moves the oldest queued job to downloading and returns it, or
	// ErrNotFound when the queue is empty.
	ClaimNext(ctx context.Context) (*Download, error)
	UpdateProgress(ctx context.Context, id string, progress float64, statusMessage string) error
	MarkCompleted(ctx context.Context, id string, targetPath string) error
	MarkFailed(ctx context.Context, id string, errMsg string) error
	MarkCanceled(ctx context.Context, id string) error
	// CancelQueued cancels id only if it is still queued; false means the worker already claimed it.
	CancelQueued(ctx context.Context, id string) (bool, error)
	// Requeue puts a failed or canceled job back at the end of the queue; false means it wasn't
	// in a retryable state.
	Requeue(ctx context.Context, id string) (bool, error)
	// RecoverInterrupted handles jobs left in downloading by a crash or restart: each goes back
	// on the queue (with requeueNote as its status message) and one more attempt counted, unless
	// it has already used maxAttempts, in which case it fails with errMsg.
	RecoverInterrupted(ctx context.Context, maxAttempts int, requeueNote, errMsg string) (requeued, failed int64, err error)
	// DeleteFinished removes every completed, failed and canceled job.
	DeleteFinished(ctx context.Context) (int64, error)
	CountByStatus(ctx context.Context, status DownloadStatus) (int64, error)
}

// FinishedDownloadStatuses are the terminal states a job can't leave except via Requeue.
var FinishedDownloadStatuses = []DownloadStatus{
	DownloadStatusCompleted, DownloadStatusError, DownloadStatusCanceled,
}
