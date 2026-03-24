// Copyright 2019, 2021, 2022, 2023, 2024 The Alpaca Authors
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
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
)

var tlsClientConfig *tls.Config

type proxyAuthenticator interface {
	do(req *http.Request, rt http.RoundTripper) (*http.Response, error)
}

type ProxyHandler struct {
	transport *http.Transport
	auth      proxyAuthenticator
	block     func(string)
}

type proxyFunc func(*http.Request) (*url.URL, error)

func NewProxyHandler(auth proxyAuthenticator, proxy proxyFunc, block func(string)) ProxyHandler {
	tr := &http.Transport{Proxy: proxy, TLSClientConfig: tlsClientConfig}
	return ProxyHandler{tr, auth, block}
}

func (ph ProxyHandler) WrapHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Pass CONNECT requests and absolute-form URIs to the ProxyHandler.
		// If the request URL has a scheme, it is an absolute-form URI
		// (RFC 7230 Section 5.3.2).
		if req.Method == http.MethodConnect || req.URL.Scheme != "" {
			ph.ServeHTTP(w, req)
			return
		}
		// The request URI is an origin-form or asterisk-form target which we
		// handle as an origin server (RFC 7230 5.3). authority-form URIs
		// are only for CONNECT, which has already been dispatched to the
		// ProxyHandler.
		next.ServeHTTP(w, req)
	})
}

func (ph ProxyHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	deleteRequestHeaders(req)
	if req.Method == http.MethodConnect {
		ph.handleConnect(w, req)
	} else {
		ph.proxyRequest(w, req, ph.auth)
	}
}

func (ph ProxyHandler) handleConnect(w http.ResponseWriter, req *http.Request) {
	// Establish a connection to the server, or an upstream proxy.
	logger := loggerFromContext(req.Context())
	proxy, err := ph.transport.Proxy(req)
	if err != nil {
		logger.Error("Error finding proxy for request", "error", err)
	}
	logger.Log(req.Context(), LevelTrace, "handleConnect",
		"host", req.Host, "proxy", proxy)
	var server net.Conn
	if proxy == nil {
		server, err = connectDirect(req)
	} else {
		server, err = connectViaProxy(req, proxy, ph.auth)
		var oe *net.OpError
		if errors.As(err, &oe) && oe.Op == "proxyconnect" {
			logger.Warn("Temporarily blocking proxy", "proxy", proxy.Host)
			ph.block(proxy.Host)
		}
	}
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	closeInDefer := true
	defer func() {
		if closeInDefer {
			_ = server.Close()
		}
	}()
	// Take over the connection back to the client by hijacking the ResponseWriter.
	h, ok := w.(http.Hijacker)
	if !ok {
		logger.Error("Error hijacking response writer")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	client, _, err := h.Hijack()
	if err != nil {
		logger.Error("Error hijacking connection", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer func() {
		if closeInDefer {
			_ = client.Close()
		}
	}()
	// Write the response directly to the client connection. If we use Go's ResponseWriter, it
	// will automatically insert a Content-Length header, which is not allowed in a 2xx CONNECT
	// response (see https://tools.ietf.org/html/rfc7231#section-4.3.6).
	var resp []byte
	if req.ProtoAtLeast(1, 1) {
		resp = []byte("HTTP/1.1 200 Connection Established\r\n\r\n")
	} else {
		resp = []byte("HTTP/1.0 200 Connection Established\r\n\r\n")
	}
	if _, err := client.Write(resp); err != nil {
		logger.Error("Error writing response", "error", err)
		return
	}
	logger.Log(req.Context(), LevelTrace, "CONNECT tunnel established", "host", req.Host)
	// Kick off goroutines to copy data in each direction. Whichever goroutine finishes first
	// will close the Reader for the other goroutine, forcing any blocked copy to unblock. This
	// prevents any goroutine from blocking indefinitely (which will leak a file descriptor).
	closeInDefer = false
	go func() { _, _ = io.Copy(server, client); _ = server.Close() }()
	go func() { _, _ = io.Copy(client, server); _ = client.Close() }()
}

func connectDirect(req *http.Request) (net.Conn, error) {
	loggerFromContext(req.Context()).Log(req.Context(), LevelTrace,
		"Dialling host directly", "host", req.Host)
	server, err := net.Dial("tcp", req.Host)
	if err != nil {
		logger := loggerFromContext(req.Context())
		logger.Error("Error dialling host", "host", req.Host, "error", err)
	}
	return server, err
}

func connectViaProxy(req *http.Request, proxy *url.URL, auth proxyAuthenticator) (net.Conn, error) {
	logger := loggerFromContext(req.Context())
	logger.Log(req.Context(), LevelTrace, "Dialling via proxy",
		"proxy", proxy.Host, "host", req.Host)
	var tr transport
	defer func() { _ = tr.Close() }()
	if err := tr.dial(proxy); err != nil {
		logger.Error("Error dialling proxy", "proxy", proxy.Host, "error", err)
		return nil, err
	}
	var resp *http.Response
	var err error
	if pa, ok := auth.(preemptiveAuthenticator); ok && pa.hasAuth(proxy.Hostname()) {
		// Cache hit - skip the unauthenticated round-trip.
		resp, err = auth.do(req, &tr)
		if err != nil {
			return nil, err
		}
		logger.Debug("Got response (preemptive auth)", "status", resp.Status)
	} else {
		resp, err = tr.RoundTrip(req)
		if err != nil {
			logger.Error("Error reading CONNECT response", "error", err)
			return nil, err
		} else if resp.StatusCode == http.StatusProxyAuthRequired && auth != nil {
			logger.Debug("Got response, retrying with auth", "status", resp.Status)
			_ = resp.Body.Close()
			if err := tr.dial(proxy); err != nil {
				logger.Error("Error re-dialling proxy", "proxy", proxy.Host, "error", err)
				return nil, err
			}
			resp, err = auth.do(req, &tr)
			if err != nil {
				return nil, err
			}
			logger.Debug("Got response", "status", resp.Status)
		}
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected response status: %s", resp.Status)
	}
	return tr.hijack(), nil
}

func (ph ProxyHandler) proxyRequest(w http.ResponseWriter, req *http.Request, auth proxyAuthenticator) {
	// Make a copy of the request body, in case we have to replay it (for authentication)
	var buf bytes.Buffer
	logger := loggerFromContext(req.Context())
	logger.Log(req.Context(), LevelTrace, "proxyRequest",
		"method", req.Method, "url", req.URL, "content_length", req.ContentLength)
	if n, err := io.Copy(&buf, req.Body); err != nil {
		logger.Error("Error copying request body",
			"got", n, "expected", req.ContentLength, "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	rd := bytes.NewReader(buf.Bytes())
	logger.Log(req.Context(), LevelTrace, "Request body buffered", "bytes", buf.Len())
	req.Body = io.NopCloser(rd)
	var resp *http.Response
	var err error
	if pa, ok := auth.(preemptiveAuthenticator); ok && pa.hasAuth(proxyHostFromReq(req, ph.transport)) {
		// Cache hit - skip the unauthenticated round-trip.
		resp, err = auth.do(req, ph.transport)
		if err != nil {
			logger.Error("Error forwarding request (preemptive auth)", "error", err)
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		logger.Debug("Got response (preemptive auth)", "status", resp.Status)
	} else {
		resp, err = ph.transport.RoundTrip(req)
		if err != nil {
			logger.Error("Error forwarding request", "error", err)
			w.WriteHeader(http.StatusBadGateway)
			var oe *net.OpError
			if errors.As(err, &oe) && oe.Op == "proxyconnect" {
				proxy, err := ph.transport.Proxy(req)
				if err != nil {
					logger.Error("Proxy connect error to unknown proxy", "error", err)
					return
				}
				logger.Warn("Temporarily blocking proxy", "proxy", proxy.Host)
				ph.block(proxy.Host)
			}
			return
		}
		if resp.StatusCode == http.StatusProxyAuthRequired && auth != nil {
			_ = resp.Body.Close()
			logger.Debug("Got response, retrying with auth", "status", resp.Status)
			_, err = rd.Seek(0, io.SeekStart)
			if err != nil {
				logger.Error("Error while seeking to start of request body", "error", err)
			} else {
				req.Body = io.NopCloser(rd)
				resp, err = auth.do(req, ph.transport)
				if err != nil {
					logger.Error("Error forwarding request (with auth)", "error", err)
					w.WriteHeader(http.StatusBadGateway)
					return
				}
			}
			logger.Debug("Got response", "status", resp.Status)
		}
	}
	defer func() { _ = resp.Body.Close() }()
	copyResponseHeaders(w, resp)
	w.WriteHeader(resp.StatusCode)
	_, err = io.Copy(w, resp.Body)
	if err != nil {
		// The response status has already been sent, so if copying fails, we can't return
		// an error status to the client.  Instead, log the error.
		logger.Error("Error copying response body", "error", err)
		return
	}
}

func proxyHostFromReq(req *http.Request, tr *http.Transport) string {
	proxyURL, err := tr.Proxy(req)
	if err != nil || proxyURL == nil {
		return ""
	}
	return proxyURL.Hostname()
}

func deleteConnectionTokens(header http.Header) {
	// Remove any header field(s) with the same name as a connection token (see
	// https://tools.ietf.org/html/rfc2616#section-14.10)
	if values, ok := header["Connection"]; ok {
		for _, value := range values {
			if value == "close" {
				continue
			}
			tokens := strings.Split(value, ",")
			for _, token := range tokens {
				header.Del(strings.TrimSpace(token))
			}
		}
	}
}

func deleteRequestHeaders(req *http.Request) {
	// Delete hop-by-hop headers (see https://tools.ietf.org/html/rfc2616#section-13.5.1)
	deleteConnectionTokens(req.Header)
	req.Header.Del("Connection")
	req.Header.Del("Keep-Alive")
	req.Header.Del("Proxy-Authorization")
	req.Header.Del("TE")
	req.Header.Del("Upgrade")
}

func copyResponseHeaders(w http.ResponseWriter, resp *http.Response) {
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	// Delete hop-by-hop headers (see https://tools.ietf.org/html/rfc2616#section-13.5.1)
	deleteConnectionTokens(w.Header())
	w.Header().Del("Connection")
	w.Header().Del("Keep-Alive")
	w.Header().Del("Proxy-Authenticate")
	w.Header().Del("Trailer")
	w.Header().Del("Transfer-Encoding")
	w.Header().Del("Upgrade")
}
