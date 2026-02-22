package httpapi

import (
	"fmt"
	"net/http"
)

type statusCapturingResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusCapturingResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusCapturingResponseWriter) Write(p []byte) (int, error) {
	// Match net/http default behavior: implicit 200 on first write.
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(p)
}

func serverErrorLoggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sw := &statusCapturingResponseWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		if sw.status >= 500 {
			logMsg(r.Context(), fmt.Sprintf("http %s %s -> %d", r.Method, r.URL.Path, sw.status))
		}
	})
}
