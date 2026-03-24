//go:build trace

package main

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// devResponseWriter wraps http.ResponseWriter to capture output metrics.
type devResponseWriter struct {
	http.ResponseWriter
	status       int
	bytesWritten int64
	headersSent  bool
}

func (w *devResponseWriter) WriteHeader(status int) {
	if !w.headersSent {
		w.status = status
		w.headersSent = true
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *devResponseWriter) Write(b []byte) (int, error) {
	if !w.headersSent {
		w.status = http.StatusOK
		w.headersSent = true
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytesWritten += int64(n)
	return n, err
}

func (w *devResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := w.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, fmt.Errorf("upstream ResponseWriter does not implement http.Hijacker")
}

func (w *devResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func devModeRule(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		logger := loggerFromContext(req.Context())

		host := req.Host
		if h, _, err := net.SplitHostPort(req.Host); err == nil {
			host = h
		}

		var contentLength int64
		if req.ContentLength > 0 {
			contentLength = req.ContentLength
		}

		logger.Log(req.Context(), LevelTrace, "devModeRule request",
			"method", req.Method,
			"host", host,
			"path", req.URL.Path,
			"scheme", req.URL.Scheme,
			"content_length", contentLength,
			"headers_count", len(req.Header),
			"transfer_encoding", strings.Join(req.TransferEncoding, ","),
			"remote_addr", req.RemoteAddr,
		)

		dw := &devResponseWriter{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()

		next.ServeHTTP(dw, req)

		duration := time.Since(start)

		logger.Log(req.Context(), LevelTrace, "devModeRule response",
			"method", req.Method,
			"host", host,
			"path", req.URL.Path,
			"status", dw.status,
			"duration", duration,
			"bytes_written", dw.bytesWritten,
			"content_length_in", contentLength,
			"response_headers_count", len(dw.Header()),
		)
	})
}
