package httpapi

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
)

type statusCapturingResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusCapturingResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
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

func (w *statusCapturingResponseWriter) Flush() {
	f, ok := w.ResponseWriter.(http.Flusher)
	if !ok {
		return
	}
	f.Flush()
}

func (w *statusCapturingResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("hijacker not supported")
	}
	return h.Hijack()
}

func (w *statusCapturingResponseWriter) Push(target string, opts *http.PushOptions) error {
	p, ok := w.ResponseWriter.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}
	return p.Push(target, opts)
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
