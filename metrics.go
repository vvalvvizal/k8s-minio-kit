package main

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"
)

// metrics collects a small set of HTTP counters/gauges and renders them in the
// Prometheus text exposition format. Kept dependency-free on purpose so the
// distroless static build stays unchanged.
type metrics struct {
	mu       sync.Mutex
	reqTotal map[reqKey]uint64
	durSum   map[string]float64
	durCount map[string]uint64
	inFlight int64
}

type reqKey struct {
	handler string
	method  string
	code    string
}

func newMetrics() *metrics {
	return &metrics{
		reqTotal: map[reqKey]uint64{},
		durSum:   map[string]float64{},
		durCount: map[string]uint64{},
	}
}

// statusRecorder captures the response status code so it can be labeled.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.wroteHeader {
		r.status = code
		r.wroteHeader = true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		r.status = http.StatusOK
		r.wroteHeader = true
	}
	return r.ResponseWriter.Write(b)
}

// instrument wraps a handler, recording request counts, in-flight gauge and
// request duration under the given logical handler name.
func (m *metrics) instrument(handler string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		m.inFlight++
		m.mu.Unlock()

		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next(rec, r)
		elapsed := time.Since(start).Seconds()

		m.mu.Lock()
		m.inFlight--
		m.reqTotal[reqKey{handler, r.Method, strconv.Itoa(rec.status)}]++
		m.durSum[handler] += elapsed
		m.durCount[handler]++
		m.mu.Unlock()
	}
}

func (m *metrics) handler(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")

	keys := make([]reqKey, 0, len(m.reqTotal))
	for k := range m.reqTotal {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].handler != keys[j].handler {
			return keys[i].handler < keys[j].handler
		}
		if keys[i].method != keys[j].method {
			return keys[i].method < keys[j].method
		}
		return keys[i].code < keys[j].code
	})

	fmt.Fprintln(w, "# HELP storage_http_requests_total Total HTTP requests handled.")
	fmt.Fprintln(w, "# TYPE storage_http_requests_total counter")
	for _, k := range keys {
		fmt.Fprintf(w, "storage_http_requests_total{handler=%q,method=%q,code=%q} %d\n",
			k.handler, k.method, k.code, m.reqTotal[k])
	}

	fmt.Fprintln(w, "# HELP storage_http_in_flight_requests Current number of in-flight HTTP requests.")
	fmt.Fprintln(w, "# TYPE storage_http_in_flight_requests gauge")
	fmt.Fprintf(w, "storage_http_in_flight_requests %d\n", m.inFlight)

	handlers := make([]string, 0, len(m.durCount))
	for h := range m.durCount {
		handlers = append(handlers, h)
	}
	sort.Strings(handlers)

	fmt.Fprintln(w, "# HELP storage_http_request_duration_seconds_sum Total time spent handling requests.")
	fmt.Fprintln(w, "# TYPE storage_http_request_duration_seconds_sum counter")
	for _, h := range handlers {
		fmt.Fprintf(w, "storage_http_request_duration_seconds_sum{handler=%q} %g\n", h, m.durSum[h])
	}

	fmt.Fprintln(w, "# HELP storage_http_request_duration_seconds_count Number of requests observed for duration.")
	fmt.Fprintln(w, "# TYPE storage_http_request_duration_seconds_count counter")
	for _, h := range handlers {
		fmt.Fprintf(w, "storage_http_request_duration_seconds_count{handler=%q} %d\n", h, m.durCount[h])
	}
}
