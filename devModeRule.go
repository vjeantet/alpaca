//go:build !trace

package main

import (
	"net/http"
)

func devModeRule(next http.Handler) http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		next.ServeHTTP(w, req)
	})
}
