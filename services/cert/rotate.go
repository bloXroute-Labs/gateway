package cert

import (
	"context"
	"errors"
	"time"

	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	"github.com/bloXroute-Labs/bxcommon-go/sdnsdk"
)

const rotateInterval = 24 * time.Hour

// Rotate periodically rotates SSL certificates for the SDN connection.
func Rotate(ctx context.Context, sdn sdnsdk.SDNHTTP) error {
	ticker := time.NewTicker(rotateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			log.Debug("rotating SSL certificates...")
			if err := sdn.RotateCertificate(ctx); err != nil {
				if errors.Is(err, context.Canceled) {
					return nil
				}
				log.Errorf("failed to rotate SSL certificates: %v", err)
			}
		case <-ctx.Done():
			return nil
		}
	}
}
