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

func NewDownloadRepository(ctx context.Context, db dbx.Builder) model.DownloadRepository {
	r := &downloadRepository{}
	r.ctx = ctx
	r.db = db
	r.tableName = "download"
	return r
}

func (r *downloadRepository) Enqueue(d *model.Download) error {
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
	_, err := r.executeSQL(ins)
	return err
}

func (r *downloadRepository) Get(id string) (*model.Download, error) {
	sel := r.newSelect().Where(Eq{"id": id}).Columns("*")
	res := model.Download{}
	err := r.queryOne(sel, &res)
	return &res, err
}

func (r *downloadRepository) GetAll(options ...model.QueryOptions) (model.Downloads, error) {
	sel := r.newSelect(options...).Columns("*")
	res := model.Downloads{}
	err := r.queryAll(sel, &res)
	return res, err
}

func (r *downloadRepository) DequeueBatch(n int) (model.Downloads, error) {
	sel := Select("*").From(r.tableName).
		Where(Eq{"status": model.DownloadStatusQueued}).
		OrderBy("created_at ASC").
		Limit(uint64(n))
	var res model.Downloads
	err := r.queryAll(sel, &res)
	return res, err
}

func (r *downloadRepository) UpdateProgress(id string, progress float64, statusMessage string) error {
	upd := Update(r.tableName).
		Set("status", model.DownloadStatusDownloading).
		Set("progress", progress).
		Set("status_message", statusMessage).
		Set("updated_at", time.Now()).
		Where(Eq{"id": id})
	_, err := r.executeSQL(upd)
	return err
}

func (r *downloadRepository) MarkStarted(id string) error {
	now := time.Now()
	upd := Update(r.tableName).
		Set("status", model.DownloadStatusDownloading).
		Set("started_at", now).
		Set("updated_at", now).
		Where(Eq{"id": id})
	_, err := r.executeSQL(upd)
	return err
}

func (r *downloadRepository) MarkCompleted(id string, targetPath string) error {
	now := time.Now()
	upd := Update(r.tableName).
		Set("status", model.DownloadStatusCompleted).
		Set("progress", 1).
		Set("target_path", targetPath).
		Set("completed_at", now).
		Set("updated_at", now).
		Where(Eq{"id": id})
	_, err := r.executeSQL(upd)
	return err
}

func (r *downloadRepository) MarkFailed(id string, errMsg string) error {
	now := time.Now()
	upd := Update(r.tableName).
		Set("status", model.DownloadStatusError).
		Set("error", errMsg).
		Set("attempts", Expr("attempts + 1")).
		Set("updated_at", now).
		Where(Eq{"id": id})
	_, err := r.executeSQL(upd)
	return err
}

func (r *downloadRepository) MarkCanceled(id string) error {
	upd := Update(r.tableName).
		Set("status", model.DownloadStatusCanceled).
		Set("updated_at", time.Now()).
		Where(Eq{"id": id})
	_, err := r.executeSQL(upd)
	return err
}

func (r *downloadRepository) CountByStatus(status model.DownloadStatus) (int64, error) {
	var res struct{ Count int64 }
	err := r.queryOne(Select("count(*) as count").From(r.tableName).Where(Eq{"status": status}), &res)
	return res.Count, err
}

var _ model.DownloadRepository = (*downloadRepository)(nil)
