package utils

import (
	"context"
	"os"
	"time"

	log "github.com/bloXroute-Labs/gateway/v2/logger"
)

// TriggerOnFileChanged triggers the given function when the file changes
func TriggerOnFileChanged(ctx context.Context, filename string, f func()) {
	var lastFileModified time.Time

	t := time.NewTicker(1 * time.Second)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				fileInfo, err := os.Stat(filename)
				if err != nil {
					log.Errorf("failed to state file %s: %s", filename, err)
					continue
				}

				if !lastFileModified.Before(fileInfo.ModTime()) {
					continue
				}

				lastFileModified = fileInfo.ModTime()
				f()
			}
		}
	}()
}
