package downloader

import "github.com/google/wire"

var Set = wire.NewSet(
	NewRegistry,
	NewService,
	NewWorker,
)
