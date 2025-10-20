package presence

import (
	"context"
	"time"

	"github.com/rs/zerolog"
)

func Start(ctx context.Context, log zerolog.Logger) {
	t := time.NewTicker(10 * time.Second)
	log.Info().Msg("presence agent: started (stub)")
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			log.Info().Msg("presence: heartbeat ok")
		}
	}
}
