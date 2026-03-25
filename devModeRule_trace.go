//go:build trace

package main

import (
	"net"
	"net/http"
	"strings"
	"time"
)

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

		start := time.Now()

		next.ServeHTTP(w, req)

		duration := time.Since(start)

		logger.Log(req.Context(), LevelTrace, "devModeRule response",
			"method", req.Method,
			"host", host,
			"duration", duration,
		)
	})
}
