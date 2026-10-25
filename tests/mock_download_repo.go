package tests

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/id"
)

type MockDownloadRepo struct {
	model.DownloadRepository
	mu   sync.Mutex
	Data map[string]model.Download // keyed by ID
	Err  error
}

func CreateMockDownloadRepo() *MockDownloadRepo {
	return &MockDownloadRepo{Data: map[string]model.Download{}}
}

func (m *MockDownloadRepo) Enqueue(_ context.Context, d *model.Download) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return m.Err
	}
	if d.ID == "" {
		d.ID = id.NewRandom()
	}
	now := time.Now()
	d.Status = model.DownloadStatusQueued
	d.CreatedAt = now
	d.UpdatedAt = now
	m.Data[d.ID] = *d
	return nil
}

func (m *MockDownloadRepo) Get(_ context.Context, downloadID string) (*model.Download, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return nil, m.Err
	}
	if d, ok := m.Data[downloadID]; ok {
		return &d, nil
	}
	return nil, model.ErrNotFound
}

func (m *MockDownloadRepo) GetAll(_ context.Context, _ ...model.QueryOptions) (model.Downloads, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return nil, m.Err
	}
	res := make(model.Downloads, 0, len(m.Data))
	for _, d := range m.Data {
		res = append(res, d)
	}
	slices.SortFunc(res, func(a, b model.Download) int { return a.CreatedAt.Compare(b.CreatedAt) })
	return res, nil
}

func (m *MockDownloadRepo) ClaimNext(_ context.Context) (*model.Download, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return nil, m.Err
	}
	var next *model.Download
	for _, d := range m.Data {
		if d.Status == model.DownloadStatusQueued && (next == nil || d.CreatedAt.Before(next.CreatedAt)) {
			next = &d
		}
	}
	if next == nil {
		return nil, model.ErrNotFound
	}
	now := time.Now()
	next.Status = model.DownloadStatusDownloading
	next.StartedAt = &now
	next.UpdatedAt = now
	m.Data[next.ID] = *next
	return next, nil
}

func (m *MockDownloadRepo) UpdateProgress(_ context.Context, downloadID string, progress float64, statusMessage string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return m.Err
	}
	if d, ok := m.Data[downloadID]; ok {
		d.Status = model.DownloadStatusDownloading
		d.Progress = progress
		d.StatusMessage = statusMessage
		d.UpdatedAt = time.Now()
		m.Data[downloadID] = d
	}
	return nil
}

func (m *MockDownloadRepo) MarkCompleted(_ context.Context, downloadID string, targetPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return m.Err
	}
	if d, ok := m.Data[downloadID]; ok {
		now := time.Now()
		d.Status = model.DownloadStatusCompleted
		d.Progress = 1
		d.TargetPath = targetPath
		d.CompletedAt = &now
		d.UpdatedAt = now
		m.Data[downloadID] = d
	}
	return nil
}

func (m *MockDownloadRepo) MarkFailed(_ context.Context, downloadID string, errMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return m.Err
	}
	if d, ok := m.Data[downloadID]; ok {
		d.Status = model.DownloadStatusError
		d.Error = errMsg
		d.Attempts++
		d.UpdatedAt = time.Now()
		m.Data[downloadID] = d
	}
	return nil
}

func (m *MockDownloadRepo) MarkCanceled(_ context.Context, downloadID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return m.Err
	}
	if d, ok := m.Data[downloadID]; ok {
		d.Status = model.DownloadStatusCanceled
		d.UpdatedAt = time.Now()
		m.Data[downloadID] = d
	}
	return nil
}

func (m *MockDownloadRepo) CancelQueued(_ context.Context, downloadID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return false, m.Err
	}
	d, ok := m.Data[downloadID]
	if !ok || d.Status != model.DownloadStatusQueued {
		return false, nil
	}
	d.Status = model.DownloadStatusCanceled
	d.UpdatedAt = time.Now()
	m.Data[downloadID] = d
	return true, nil
}

func (m *MockDownloadRepo) Requeue(_ context.Context, downloadID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return false, m.Err
	}
	d, ok := m.Data[downloadID]
	if !ok || (d.Status != model.DownloadStatusError && d.Status != model.DownloadStatusCanceled) {
		return false, nil
	}
	now := time.Now()
	d.Status = model.DownloadStatusQueued
	d.Progress = 0
	d.StatusMessage = ""
	d.Error = ""
	d.TargetPath = ""
	d.StartedAt = nil
	d.CompletedAt = nil
	d.CreatedAt = now
	d.UpdatedAt = now
	m.Data[downloadID] = d
	return true, nil
}

func (m *MockDownloadRepo) RecoverInterrupted(_ context.Context, maxAttempts int, requeueNote, errMsg string) (int64, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return 0, 0, m.Err
	}
	var requeued, failed int64
	for id, d := range m.Data {
		if d.Status != model.DownloadStatusDownloading {
			continue
		}
		if d.Attempts < maxAttempts-1 {
			d.Status = model.DownloadStatusQueued
			d.Progress = 0
			d.StatusMessage = requeueNote
			d.StartedAt = nil
			requeued++
		} else {
			d.Status = model.DownloadStatusError
			d.Error = errMsg
			failed++
		}
		d.Attempts++
		m.Data[id] = d
	}
	return requeued, failed, nil
}

func (m *MockDownloadRepo) DeleteFinished(_ context.Context) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return 0, m.Err
	}
	var n int64
	for id, d := range m.Data {
		if slices.Contains(model.FinishedDownloadStatuses, d.Status) {
			delete(m.Data, id)
			n++
		}
	}
	return n, nil
}

func (m *MockDownloadRepo) CountByStatus(_ context.Context, status model.DownloadStatus) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return 0, m.Err
	}
	var count int64
	for _, d := range m.Data {
		if d.Status == status {
			count++
		}
	}
	return count, nil
}

var _ model.DownloadRepository = (*MockDownloadRepo)(nil)
