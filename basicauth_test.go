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
	"context"
	"encoding/base64"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeRoundTripper struct {
	lastReq *http.Request
}

func (f *fakeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	f.lastReq = req
	return &http.Response{StatusCode: http.StatusOK}, nil
}

func TestBasicAuthSetsHeader(t *testing.T) {
	ks := newMockBasicKeystore()
	_ = ks.set("proxy.corp.com", "admin:secret")
	store := &basicCredentialStore{store: ks}
	auth := newBasicAuthenticator(store)

	proxyURL, _ := url.Parse("http://proxy.corp.com:8080")
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	ctx := context.WithValue(req.Context(), contextKeyProxy, proxyURL)
	req = req.WithContext(ctx)

	rt := &fakeRoundTripper{}
	_, err := auth.do(req, rt)
	require.NoError(t, err)

	expected := "Basic " + base64.StdEncoding.EncodeToString([]byte("admin:secret"))
	assert.Equal(t, expected, rt.lastReq.Header.Get("Proxy-Authorization"))
}

func TestBasicAuthNoHeaderWhenNoCredential(t *testing.T) {
	ks := newMockBasicKeystore()
	store := &basicCredentialStore{store: ks}
	auth := newBasicAuthenticator(store)

	proxyURL, _ := url.Parse("http://unknown.proxy.com:8080")
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	ctx := context.WithValue(req.Context(), contextKeyProxy, proxyURL)
	req = req.WithContext(ctx)

	rt := &fakeRoundTripper{}
	_, err := auth.do(req, rt)
	require.NoError(t, err)

	assert.Empty(t, rt.lastReq.Header.Get("Proxy-Authorization"))
}

func TestBasicAuthNoProxyContext(t *testing.T) {
	ks := newMockBasicKeystore()
	_ = ks.set("*", "default:pass")
	store := &basicCredentialStore{store: ks}
	auth := newBasicAuthenticator(store)

	req, _ := http.NewRequest("GET", "http://example.com", nil)

	rt := &fakeRoundTripper{}
	_, err := auth.do(req, rt)
	require.NoError(t, err)

	expected := "Basic " + base64.StdEncoding.EncodeToString([]byte("default:pass"))
	assert.Equal(t, expected, rt.lastReq.Header.Get("Proxy-Authorization"))
}

func TestNewBasicAuthenticatorNilStore(t *testing.T) {
	assert.Nil(t, newBasicAuthenticator(nil))
}

func TestBasicAuthWithFlagCreds(t *testing.T) {
	store := &basicCredentialStore{flagCreds: "flag-user:flag-pass"}
	auth := newBasicAuthenticator(store)

	proxyURL, _ := url.Parse("http://any.proxy.com:8080")
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	ctx := context.WithValue(req.Context(), contextKeyProxy, proxyURL)
	req = req.WithContext(ctx)

	rt := &fakeRoundTripper{}
	_, err := auth.do(req, rt)
	require.NoError(t, err)

	expected := "Basic " + base64.StdEncoding.EncodeToString([]byte("flag-user:flag-pass"))
	assert.Equal(t, expected, rt.lastReq.Header.Get("Proxy-Authorization"))
}

func TestBasicAuthGlobMatchedProxy(t *testing.T) {
	ks := newMockBasicKeystore()
	_ = ks.set("*.corp.com", "glob-user:glob-pass")
	store := &basicCredentialStore{store: ks}
	auth := newBasicAuthenticator(store)

	proxyURL, _ := url.Parse("http://proxy.corp.com:3128")
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	ctx := context.WithValue(req.Context(), contextKeyProxy, proxyURL)
	req = req.WithContext(ctx)

	rt := &fakeRoundTripper{}
	_, err := auth.do(req, rt)
	require.NoError(t, err)

	expected := "Basic " + base64.StdEncoding.EncodeToString([]byte("glob-user:glob-pass"))
	assert.Equal(t, expected, rt.lastReq.Header.Get("Proxy-Authorization"))
}

func TestBasicAuthDifferentProxiesDifferentCreds(t *testing.T) {
	ks := newMockBasicKeystore()
	_ = ks.set("proxy-a.com", "user-a:pass-a")
	_ = ks.set("proxy-b.com", "user-b:pass-b")
	store := &basicCredentialStore{store: ks}
	auth := newBasicAuthenticator(store)

	tests := []struct {
		proxyHost string
		wantCred  string
	}{
		{"proxy-a.com", "user-a:pass-a"},
		{"proxy-b.com", "user-b:pass-b"},
	}
	for _, tt := range tests {
		proxyURL, _ := url.Parse("http://" + tt.proxyHost + ":8080")
		req, _ := http.NewRequest("GET", "http://example.com", nil)
		ctx := context.WithValue(req.Context(), contextKeyProxy, proxyURL)
		req = req.WithContext(ctx)

		rt := &fakeRoundTripper{}
		_, err := auth.do(req, rt)
		require.NoError(t, err)

		expected := "Basic " + base64.StdEncoding.EncodeToString([]byte(tt.wantCred))
		assert.Equal(t, expected, rt.lastReq.Header.Get("Proxy-Authorization"),
			"wrong credential for proxy %s", tt.proxyHost)
	}
}
