package tests

import (
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

func (m *MockDownloadRepo) Enqueue(d *model.Download) error {
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

func (m *MockDownloadRepo) Get(downloadID string) (*model.Download, error) {
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

func (m *MockDownloadRepo) GetAll(...model.QueryOptions) (model.Downloads, error) {
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

func (m *MockDownloadRepo) DequeueBatch(n int) (model.Downloads, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return nil, m.Err
	}
	var res model.Downloads
	for _, d := range m.Data {
		if d.Status == model.DownloadStatusQueued {
			res = append(res, d)
		}
	}
	slices.SortFunc(res, func(a, b model.Download) int { return a.CreatedAt.Compare(b.CreatedAt) })
	if len(res) > n {
		res = res[:n]
	}
	return res, nil
}

func (m *MockDownloadRepo) UpdateProgress(downloadID string, progress float64, statusMessage string) error {
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

func (m *MockDownloadRepo) MarkStarted(downloadID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return m.Err
	}
	if d, ok := m.Data[downloadID]; ok {
		now := time.Now()
		d.Status = model.DownloadStatusDownloading
		d.StartedAt = &now
		d.UpdatedAt = now
		m.Data[downloadID] = d
	}
	return nil
}

func (m *MockDownloadRepo) MarkCompleted(downloadID string, targetPath string) error {
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

func (m *MockDownloadRepo) MarkFailed(downloadID string, errMsg string) error {
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

func (m *MockDownloadRepo) MarkCanceled(downloadID string) error {
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

func (m *MockDownloadRepo) CountByStatus(status model.DownloadStatus) (int64, error) {
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
