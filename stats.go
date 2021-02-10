package sqsjfr

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
)

// Stats represents sqsjfr stats.
type Stats struct {
	Entries struct {
		Registered int64 `json:"registered"`
	} `json:"entries"`
	Invocations struct {
		Succeeded int64 `json:"succeeded"`
		Failed    int64 `json:"failed"`
	} `json:"invocations"`
}

func (app *App) runStatsServer() error {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-type", "application/json")
		enc := json.NewEncoder(w)
		if err := enc.Encode(app.stats); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/stats/metrics", handler)
	addr := fmt.Sprintf(":%d", app.option.StatsPort)
	srv := &http.Server{
		Handler: mux,
		Addr:    addr,
	}
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	log.Printf("[info] starting up stats server on %s", l.Addr())
	return srv.Serve(l)
}
