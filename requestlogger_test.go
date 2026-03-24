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
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestLogger(t *testing.T) {
	tests := map[string]struct {
		status  int
		wrapper func(http.Handler) http.Handler
		checks  []string
	}{
		"No Status": {0, nil, []string{"status=200", "method=GET"}},
		"Given Status": {http.StatusNotFound, nil, []string{
			"status=404", "method=GET",
		}},
		"Context": {http.StatusOK, AddContextID, []string{
			"status=200", "method=GET", "request_id=1",
		}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			handler := slog.NewTextHandler(&buf, nil)
			slog.SetDefault(slog.New(handler))

			hfunc := func(w http.ResponseWriter, req *http.Request) {
				if tt.status != 0 {
					w.WriteHeader(tt.status)
				}
			}
			var h http.Handler = http.HandlerFunc(hfunc)
			h = RequestLogger(h)
			if tt.wrapper != nil {
				h = tt.wrapper(h)
			}
			server := httptest.NewServer(h)
			defer server.Close()
			_, err := http.Get(server.URL)
			require.NoError(t, err)
			output := buf.String()
			for _, check := range tt.checks {
				assert.Contains(t, output, check)
			}
		})
	}
}
