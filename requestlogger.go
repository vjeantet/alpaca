// Copyright 2019 The Alpaca Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"time"
)

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := w.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, fmt.Errorf("upstream ResponseWriter does not implement http.Hijacker")
}

type jsonLogEntry struct {
	ID          uint64 `json:"id"`
	Timestamp   string `json:"timestamp"`
	Status      int    `json:"status"`
	Method      string `json:"method"`
	URL         string `json:"url"`
	ParentProxy string `json:"parent_proxy"`
}

func RequestLoggerJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, req)
		var parentProxy string
		if value := req.Context().Value(contextKeyProxy); value != nil {
			parentProxy = value.(*url.URL).Host
		}
		var id uint64
		if value := req.Context().Value(contextKeyID); value != nil {
			id = value.(uint64)
		}
		entry := jsonLogEntry{
			ID:          id,
			Timestamp:   time.Now().UTC().Format(time.RFC3339Nano),
			Status:      sw.status,
			Method:      req.Method,
			URL:         req.URL.String(),
			ParentProxy: parentProxy,
		}
		if data, err := json.Marshal(entry); err == nil {
			fmt.Println(string(data))
		}
	})
}

func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, req)
		log.Printf(
			"[%v] %d %s %s",
			req.Context().Value(contextKeyID),
			sw.status,
			req.Method,
			req.URL,
		)
	})
}
