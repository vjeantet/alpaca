package main

import (
	"log"
	"net"
	"net/http"
)

func devModeRule(next http.Handler) http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var host string
		if h, _, err := net.SplitHostPort(req.Host); err == nil {
			host = h
		}

		log.Printf("req.Host: %s", host)
		next.ServeHTTP(w, req)
	})
}
