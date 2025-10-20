package api

import (
	"net/http"

	"slowdrip-miner/internal/config"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func Router(cfg *config.Config) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200); w.Write([]byte("ok")) })
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200); w.Write([]byte("ready")) })
	if cfg.Metrics.Enable {
		mux.Handle("/metrics", promhttp.Handler())
	}
	return mux
}
