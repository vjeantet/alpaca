package main

import (
	"net"
	"net/http"
)

func devModeRule(next http.Handler) http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var host string
		if h, _, err := net.SplitHostPort(req.Host); err == nil {
			host = h
		}

		loggerFromContext(req.Context()).Log(req.Context(), LevelTrace, "devModeRule", "host", host)
		next.ServeHTTP(w, req)
	})
}
