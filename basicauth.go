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
	"encoding/base64"
	"net/http"
	"net/url"
)

type basicAuthenticator struct {
	store *basicCredentialStore
}

func newBasicAuthenticator(store *basicCredentialStore) *basicAuthenticator {
	if store == nil {
		return nil
	}
	return &basicAuthenticator{store: store}
}

func (b *basicAuthenticator) do(req *http.Request, rt http.RoundTripper) (*http.Response, error) {
	proxyHost := ""
	if v := req.Context().Value(contextKeyProxy); v != nil {
		proxyHost = v.(*url.URL).Hostname()
	}
	creds := b.store.resolve(proxyHost)
	if creds == "" {
		return rt.RoundTrip(req)
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(creds))
	req.Header.Set("Proxy-Authorization", "Basic "+encoded)
	return rt.RoundTrip(req)
}
