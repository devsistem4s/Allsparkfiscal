package observability

import (
	"allspark.local/fiscal/internal/store"
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
)

type Metrics struct {
	Requests      atomic.Uint64
	DurationNanos atomic.Uint64
}

func (m *Metrics) Observe(start time.Time) {
	m.Requests.Add(1)
	m.DurationNanos.Add(uint64(time.Since(start)))
}
func (m *Metrics) Handler(s *store.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/metrics" {
			http.NotFound(w, r)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		fmt.Fprintf(w, "# TYPE allspark_http_requests_total counter\nallspark_http_requests_total %d\n# TYPE allspark_http_duration_seconds summary\nallspark_http_duration_seconds_sum %f\nallspark_http_duration_seconds_count %d\n", m.Requests.Load(), float64(m.DurationNanos.Load())/1e9, m.Requests.Load())
		rows, e := s.DB.Query(ctx, "SELECT state,count(*) FROM jobs WHERE state IN ('queued','processing','transmitting') GROUP BY state")
		if e != nil {
			fmt.Fprintln(w, "allspark_metrics_database_up 0")
			return
		}
		defer rows.Close()
		fmt.Fprintln(w, "allspark_metrics_database_up 1\n# TYPE allspark_jobs gauge")
		for rows.Next() {
			var state string
			var n int64
			if rows.Scan(&state, &n) == nil {
				fmt.Fprintf(w, "allspark_jobs{state=%q} %d\n", state, n)
			}
		}
	})
}
