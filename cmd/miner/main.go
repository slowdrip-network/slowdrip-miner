package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"slowdrip-miner/internal/api"
	"slowdrip-miner/internal/config"
	"slowdrip-miner/internal/logger"
	"slowdrip-miner/internal/mediamtx"
	"slowdrip-miner/internal/presence"
	"slowdrip-miner/internal/service"
)

func main() {
	cfgPath := os.Getenv("MINER_CONFIG")
	if cfgPath == "" {
		cfgPath = "configs/miner.yaml"
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	lg := logger.New(cfg.LogLevel)

	mm := mediamtx.NewClient(cfg.MediaMTX.API, lg)
	go mediamtx.StartWatcher(context.Background(), mm, cfg.MediaMTX.PollInterval)

	if cfg.Presence.Enable {
		go presence.Start(context.Background(), lg)
	}
	if cfg.Service.Enable {
		go service.Start(context.Background(), lg)
	}

	mux := api.Router(cfg)
	srv := &http.Server{
		Addr:              cfg.Miner.Listen,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	lg.Info().Msgf("miner %s listening on %s", cfg.Miner.ID, cfg.Miner.Listen)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		lg.Fatal().Err(err).Msg("server failed")
	}
}
