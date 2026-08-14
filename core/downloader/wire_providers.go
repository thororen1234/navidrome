package downloader

import "github.com/google/wire"

var Set = wire.NewSet(
	NewService,
	NewWorker,
)
