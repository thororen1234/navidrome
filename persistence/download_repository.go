package persistence

import (
	"context"
	"time"

	. "github.com/Masterminds/squirrel"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/id"
	"github.com/pocketbase/dbx"
)

type downloadRepository struct {
	sqlRepository
}

func NewDownloadRepository(db dbx.Builder) model.DownloadRepository {
	r := &downloadRepository{}
	r.db = db
	r.tableName = "download"
	return r
}

func (r *downloadRepository) Enqueue(ctx context.Context, d *model.Download) error {
	if d.ID == "" {
		d.ID = id.NewRandom()
	}
	now := time.Now()
	d.Status = model.DownloadStatusQueued
	d.CreatedAt = now
	d.UpdatedAt = now
	ins := Insert(r.tableName).
		Columns("id", "tool", "source_url", "tidal_id", "tidal_kind", "status", "progress",
			"status_message", "error", "library_id", "target_path", "requested_by", "attempts",
			"created_at", "updated_at").
		Values(d.ID, d.Tool, d.SourceURL, d.TidalID, d.TidalKind, d.Status, 0, "", "",
			d.LibraryID, "", d.RequestedBy, 0, d.CreatedAt, d.UpdatedAt)
	_, err := r.executeSQL(ctx, ins)
	return err
}

func (r *downloadRepository) Get(ctx context.Context, id string) (*model.Download, error) {
	sel := r.newSelect(ctx).Where(Eq{"id": id}).Columns("*")
	res := model.Download{}
	err := r.queryOne(ctx, sel, &res)
	return &res, err
}

func (r *downloadRepository) GetAll(ctx context.Context, options ...model.QueryOptions) (model.Downloads, error) {
	sel := r.newSelect(ctx, options...).Columns("*")
	res := model.Downloads{}
	err := r.queryAll(ctx, sel, &res)
	return res, err
}

func (r *downloadRepository) ClaimNext(ctx context.Context) (*model.Download, error) {
	now := time.Now()
	next := Select("id").From(r.tableName).
		Where(Eq{"status": model.DownloadStatusQueued}).
		OrderBy("created_at ASC").
		Limit(1)
	upd := Update(r.tableName).
		Set("status", model.DownloadStatusDownloading).
		Set("progress", 0).
		Set("status_message", "").
		Set("error", "").
		Set("started_at", now).
		Set("updated_at", now).
		Where(Expr("id = (?)", next)).
		Where(Eq{"status": model.DownloadStatusQueued}).
		Suffix("RETURNING *")
	res := model.Download{}
	if err := r.queryOne(ctx, upd, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (r *downloadRepository) UpdateProgress(ctx context.Context, id string, progress float64, statusMessage string) error {
	upd := Update(r.tableName).
		Set("status", model.DownloadStatusDownloading).
		Set("progress", progress).
		Set("status_message", statusMessage).
		Set("updated_at", time.Now()).
		Where(Eq{"id": id})
	_, err := r.executeSQL(ctx, upd)
	return err
}

func (r *downloadRepository) MarkCompleted(ctx context.Context, id string, targetPath string) error {
	now := time.Now()
	upd := Update(r.tableName).
		Set("status", model.DownloadStatusCompleted).
		Set("progress", 1).
		Set("target_path", targetPath).
		Set("completed_at", now).
		Set("updated_at", now).
		Where(Eq{"id": id})
	_, err := r.executeSQL(ctx, upd)
	return err
}

func (r *downloadRepository) MarkFailed(ctx context.Context, id string, errMsg string) error {
	now := time.Now()
	upd := Update(r.tableName).
		Set("status", model.DownloadStatusError).
		Set("error", errMsg).
		Set("attempts", Expr("attempts + 1")).
		Set("updated_at", now).
		Where(Eq{"id": id})
	_, err := r.executeSQL(ctx, upd)
	return err
}

func (r *downloadRepository) MarkCanceled(ctx context.Context, id string) error {
	upd := Update(r.tableName).
		Set("status", model.DownloadStatusCanceled).
		Set("updated_at", time.Now()).
		Where(Eq{"id": id})
	_, err := r.executeSQL(ctx, upd)
	return err
}

func (r *downloadRepository) CancelQueued(ctx context.Context, id string) (bool, error) {
	upd := Update(r.tableName).
		Set("status", model.DownloadStatusCanceled).
		Set("updated_at", time.Now()).
		Where(Eq{"id": id, "status": model.DownloadStatusQueued})
	n, err := r.executeSQL(ctx, upd)
	return n > 0, err
}

func (r *downloadRepository) Requeue(ctx context.Context, id string) (bool, error) {
	now := time.Now()
	upd := Update(r.tableName).
		Set("status", model.DownloadStatusQueued).
		Set("progress", 0).
		Set("status_message", "").
		Set("error", "").
		Set("target_path", "").
		Set("started_at", nil).
		Set("completed_at", nil).
		Set("created_at", now).
		Set("updated_at", now).
		Where(Eq{"id": id, "status": []model.DownloadStatus{model.DownloadStatusError, model.DownloadStatusCanceled}})
	n, err := r.executeSQL(ctx, upd)
	return n > 0, err
}

func (r *downloadRepository) RecoverInterrupted(ctx context.Context, maxAttempts int, requeueNote, errMsg string) (int64, int64, error) {
	now := time.Now()
	requeue := Update(r.tableName).
		Set("status", model.DownloadStatusQueued).
		Set("progress", 0).
		Set("status_message", requeueNote).
		Set("attempts", Expr("attempts + 1")).
		Set("started_at", nil).
		Set("updated_at", now).
		Where(Eq{"status": model.DownloadStatusDownloading}).
		Where(Lt{"attempts": maxAttempts - 1})
	requeued, err := r.executeSQL(ctx, requeue)
	if err != nil {
		return 0, 0, err
	}
	fail := Update(r.tableName).
		Set("status", model.DownloadStatusError).
		Set("error", errMsg).
		Set("attempts", Expr("attempts + 1")).
		Set("updated_at", now).
		Where(Eq{"status": model.DownloadStatusDownloading})
	failed, err := r.executeSQL(ctx, fail)
	return requeued, failed, err
}

func (r *downloadRepository) DeleteFinished(ctx context.Context) (int64, error) {
	return r.executeSQL(ctx, Delete(r.tableName).Where(Eq{"status": model.FinishedDownloadStatuses}))
}

func (r *downloadRepository) CountByStatus(ctx context.Context, status model.DownloadStatus) (int64, error) {
	var res struct{ Count int64 }
	err := r.queryOne(ctx, Select("count(*) as count").From(r.tableName).Where(Eq{"status": status}), &res)
	return res.Count, err
}

var _ model.DownloadRepository = (*downloadRepository)(nil)
