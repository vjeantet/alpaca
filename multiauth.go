// Copyright 2025 The Alpaca Authors
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
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sync"
)

// multiAuthenticator tries multiple authentication methods in order and caches
// which method works for each proxy host to avoid redundant retries.
type multiAuthenticator struct {
	methods []proxyAuthenticator
	cache   map[string]proxyAuthenticator
	mu      sync.RWMutex
}

// newMultiAuthenticator builds a proxyAuthenticator from the given methods,
// skipping any nil entries. Returns nil if no methods are available, returns
// the single method directly if only one is provided, or returns a
// multiAuthenticator that tries each method in order with per-proxy caching.
func newMultiAuthenticator(methods ...proxyAuthenticator) proxyAuthenticator {
	var filtered []proxyAuthenticator
	for _, m := range methods {
		if m != nil {
			filtered = append(filtered, m)
		}
	}
	switch len(filtered) {
	case 0:
		return nil
	case 1:
		return filtered[0]
	default:
		return &multiAuthenticator{
			methods: filtered,
			cache:   make(map[string]proxyAuthenticator),
		}
	}
}

func (m *multiAuthenticator) do(req *http.Request, rt http.RoundTripper) (*http.Response, error) {
	proxyHost := ""
	if value := req.Context().Value(contextKeyProxy); value != nil {
		proxyHost = value.(*url.URL).Hostname()
	}

	// Use cached auth method if we already know what works for this proxy.
	if proxyHost != "" {
		m.mu.RLock()
		cached, ok := m.cache[proxyHost]
		m.mu.RUnlock()
		if ok {
			return cached.do(req, rt)
		}
	}

	// Try each method in order until one succeeds (non-407 response).
	for i, method := range m.methods {
		resp, err := method.do(req, rt)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusProxyAuthRequired {
			// This method worked — cache it for this proxy.
			if proxyHost != "" {
				m.mu.Lock()
				m.cache[proxyHost] = method
				m.mu.Unlock()
				log.Printf("Cached auth method for proxy %s", proxyHost)
			}
			return resp, nil
		}
		// 407 — this method was rejected, try the next one.
		if i < len(m.methods)-1 {
			resp.Body.Close()
		} else {
			// Last method also failed, return the 407 as-is.
			return resp, nil
		}
	}

	return nil, fmt.Errorf("no authentication methods configured")
}
