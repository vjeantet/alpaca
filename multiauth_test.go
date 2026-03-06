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
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// staticAuth returns a fixed status code on every call.
type staticAuth struct {
	status int
	calls  int
}

func (a *staticAuth) do(req *http.Request, rt http.RoundTripper) (*http.Response, error) {
	a.calls++
	return &http.Response{
		StatusCode: a.status,
		Body:       io.NopCloser(strings.NewReader("")),
	}, nil
}

// errorAuth always returns a transport error.
type errorAuth struct{}

func (a *errorAuth) do(req *http.Request, rt http.RoundTripper) (*http.Response, error) {
	return nil, fmt.Errorf("auth transport error")
}

func reqWithProxy(proxyHost string) *http.Request {
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	if proxyHost != "" {
		proxyURL, _ := url.Parse("http://" + proxyHost + ":8080")
		ctx := context.WithValue(req.Context(), contextKeyProxy, proxyURL)
		req = req.WithContext(ctx)
	}
	return req
}

// --- newMultiAuthenticator construction ---

func TestNewMultiAuthenticatorNoMethods(t *testing.T) {
	assert.Nil(t, newMultiAuthenticator())
}

func TestNewMultiAuthenticatorAllNil(t *testing.T) {
	assert.Nil(t, newMultiAuthenticator(nil, nil, nil))
}

func TestNewMultiAuthenticatorSingle(t *testing.T) {
	a := &staticAuth{status: http.StatusOK}
	result := newMultiAuthenticator(a)
	// Single method returned directly, not wrapped in multiAuthenticator
	assert.Equal(t, a, result)
}

func TestNewMultiAuthenticatorSingleSkipsNil(t *testing.T) {
	a := &staticAuth{status: http.StatusOK}
	result := newMultiAuthenticator(nil, a, nil)
	assert.Equal(t, a, result)
}

func TestNewMultiAuthenticatorMultiple(t *testing.T) {
	a := &staticAuth{status: http.StatusOK}
	b := &staticAuth{status: http.StatusOK}
	result := newMultiAuthenticator(a, b)
	require.NotNil(t, result)
	_, isMulti := result.(*multiAuthenticator)
	assert.True(t, isMulti)
}

// --- multiAuthenticator.do ---

func TestMultiAuthFirstMethodSucceeds(t *testing.T) {
	first := &staticAuth{status: http.StatusOK}
	second := &staticAuth{status: http.StatusOK}
	auth := newMultiAuthenticator(first, second)

	resp, err := auth.do(reqWithProxy("proxy.com"), &fakeRoundTripper{})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, 1, first.calls)
	assert.Equal(t, 0, second.calls)
}

func TestMultiAuthFallsThrough407(t *testing.T) {
	first := &staticAuth{status: http.StatusProxyAuthRequired}
	second := &staticAuth{status: http.StatusOK}
	auth := newMultiAuthenticator(first, second)

	resp, err := auth.do(reqWithProxy("proxy.com"), &fakeRoundTripper{})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, 1, first.calls)
	assert.Equal(t, 1, second.calls)
}

func TestMultiAuthAllFail407(t *testing.T) {
	first := &staticAuth{status: http.StatusProxyAuthRequired}
	second := &staticAuth{status: http.StatusProxyAuthRequired}
	auth := newMultiAuthenticator(first, second)

	resp, err := auth.do(reqWithProxy("proxy.com"), &fakeRoundTripper{})
	require.NoError(t, err)
	assert.Equal(t, http.StatusProxyAuthRequired, resp.StatusCode)
}

func TestMultiAuthCachesSuccessfulMethod(t *testing.T) {
	first := &staticAuth{status: http.StatusProxyAuthRequired}
	second := &staticAuth{status: http.StatusOK}
	auth := newMultiAuthenticator(first, second)
	rt := &fakeRoundTripper{}

	// First call: first returns 407, second returns 200 -> caches second
	resp, err := auth.do(reqWithProxy("proxy.com"), rt)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Second call: cached method (second) used directly
	resp, err = auth.do(reqWithProxy("proxy.com"), rt)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, 1, first.calls, "first should only be tried once")
	assert.Equal(t, 2, second.calls, "second used on initial + cached call")
}

func TestMultiAuthCachePerProxy(t *testing.T) {
	first := &staticAuth{status: http.StatusOK}
	second := &staticAuth{status: http.StatusOK}
	auth := newMultiAuthenticator(first, second)
	rt := &fakeRoundTripper{}

	_, _ = auth.do(reqWithProxy("proxy-a.com"), rt)
	assert.Equal(t, 1, first.calls)

	// Different proxy triggers a fresh walk through the method chain
	_, _ = auth.do(reqWithProxy("proxy-b.com"), rt)
	assert.Equal(t, 2, first.calls)
}

func TestMultiAuthNoProxyContextNoCache(t *testing.T) {
	first := &staticAuth{status: http.StatusProxyAuthRequired}
	second := &staticAuth{status: http.StatusOK}
	auth := newMultiAuthenticator(first, second)
	rt := &fakeRoundTripper{}

	req, _ := http.NewRequest("GET", "http://example.com", nil)
	_, _ = auth.do(req, rt)

	req, _ = http.NewRequest("GET", "http://example.com", nil)
	_, _ = auth.do(req, rt)

	// Without proxy context, no caching happens - all methods retried
	assert.Equal(t, 2, first.calls)
	assert.Equal(t, 2, second.calls)
}

func TestMultiAuthMethodError(t *testing.T) {
	auth := newMultiAuthenticator(&errorAuth{}, &staticAuth{status: http.StatusOK})
	_, err := auth.do(reqWithProxy("proxy.com"), &fakeRoundTripper{})
	assert.Error(t, err)
}

func TestMultiAuthThreeMethods(t *testing.T) {
	first := &staticAuth{status: http.StatusProxyAuthRequired}
	second := &staticAuth{status: http.StatusProxyAuthRequired}
	third := &staticAuth{status: http.StatusOK}
	auth := newMultiAuthenticator(first, second, third)

	resp, err := auth.do(reqWithProxy("proxy.com"), &fakeRoundTripper{})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, 1, first.calls)
	assert.Equal(t, 1, second.calls)
	assert.Equal(t, 1, third.calls)
}

func TestMultiAuthNon407NonOKIsCachedToo(t *testing.T) {
	// A 403 response is not a 407, so it counts as "method works" and gets cached
	first := &staticAuth{status: http.StatusForbidden}
	second := &staticAuth{status: http.StatusOK}
	auth := newMultiAuthenticator(first, second)
	rt := &fakeRoundTripper{}

	resp, err := auth.do(reqWithProxy("proxy.com"), rt)
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	assert.Equal(t, 0, second.calls, "second should not be tried when first returns non-407")

	// Cached: first method used again
	_, err = auth.do(reqWithProxy("proxy.com"), rt)
	require.NoError(t, err)
	assert.Equal(t, 2, first.calls)
	assert.Equal(t, 0, second.calls)
}
