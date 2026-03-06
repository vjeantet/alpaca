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
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fakePwd(password string) func() ([]byte, error) {
	return func() ([]byte, error) {
		return []byte(password), nil
	}
}

func TestCredentialAddDefault(t *testing.T) {
	ks := newMockBasicKeystore()
	code := credentialAdd(ks, []string{"-u", "mylogin"}, fakePwd("mypass"))
	assert.Equal(t, 0, code)
	secret, err := ks.get("*")
	require.NoError(t, err)
	assert.Equal(t, "mylogin:mypass", secret)
}

func TestCredentialAddWithProxy(t *testing.T) {
	ks := newMockBasicKeystore()
	code := credentialAdd(ks, []string{"-u", "admin", "proxy.corp.com"}, fakePwd("s3cret"))
	assert.Equal(t, 0, code)
	secret, err := ks.get("proxy.corp.com")
	require.NoError(t, err)
	assert.Equal(t, "admin:s3cret", secret)
}

func TestCredentialAddWithGlobProxy(t *testing.T) {
	ks := newMockBasicKeystore()
	code := credentialAdd(ks, []string{"-u", "user2", "*.corp.com"}, fakePwd("pass"))
	assert.Equal(t, 0, code)
	secret, err := ks.get("*.corp.com")
	require.NoError(t, err)
	assert.Equal(t, "user2:pass", secret)
}

func TestCredentialAddMissingLogin(t *testing.T) {
	ks := newMockBasicKeystore()
	code := credentialAdd(ks, []string{}, fakePwd("pass"))
	assert.Equal(t, 1, code)
}

func TestCredentialAddLoginWithColon(t *testing.T) {
	ks := newMockBasicKeystore()
	code := credentialAdd(ks, []string{"-u", "bad:login"}, fakePwd("pass"))
	assert.Equal(t, 1, code)
}

func TestCredentialRemoveExisting(t *testing.T) {
	ks := newMockBasicKeystore()
	_ = ks.set("proxy.corp.com", "admin:pass")
	code := credentialRemove(ks, []string{"proxy.corp.com"})
	assert.Equal(t, 0, code)
	_, err := ks.get("proxy.corp.com")
	assert.Error(t, err)
}

func TestCredentialRemoveDefault(t *testing.T) {
	ks := newMockBasicKeystore()
	_ = ks.set("*", "default:pass")
	code := credentialRemove(ks, []string{})
	assert.Equal(t, 0, code)
	_, err := ks.get("*")
	assert.Error(t, err)
}

func TestCredentialRemoveNonExistent(t *testing.T) {
	ks := newMockBasicKeystore()
	code := credentialRemove(ks, []string{"nope"})
	assert.Equal(t, 1, code)
}

func TestCredentialListEmpty(t *testing.T) {
	ks := newMockBasicKeystore()
	var buf bytes.Buffer
	code := credentialList(ks, &buf)
	assert.Equal(t, 0, code)
	assert.Contains(t, buf.String(), "No credentials stored.")
}

func TestCredentialListWithEntries(t *testing.T) {
	ks := newMockBasicKeystore()
	_ = ks.set("proxy.corp.com", "admin:pass")
	_ = ks.set("*", "default:pass")
	var buf bytes.Buffer
	code := credentialList(ks, &buf)
	assert.Equal(t, 0, code)
	output := buf.String()
	assert.Contains(t, output, "proxy.corp.com")
	assert.Contains(t, output, "admin")
	assert.Contains(t, output, "(default)")
	assert.Contains(t, output, "default")
	// Password should not appear in output
	assert.NotContains(t, output, "admin:pass")
}

// --- patternLabel tests ---

func TestPatternLabel(t *testing.T) {
	tests := []struct {
		name, pattern, want string
	}{
		{"default", "*", "(default)"},
		{"specific", "proxy.corp.com", "proxy.corp.com"},
		{"glob", "*.corp.com", "*.corp.com"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, patternLabel(tt.pattern))
		})
	}
}

// --- Error path tests ---

func TestCredentialAddStoreSetError(t *testing.T) {
	ks := &errorKeystore{
		mockBasicKeystore: newMockBasicKeystore(),
		setErr:            fmt.Errorf("keychain write failed"),
	}
	code := credentialAdd(ks, []string{"-u", "admin", "proxy.com"}, fakePwd("pass"))
	assert.Equal(t, 1, code)
}

func TestCredentialAddPasswordReadError(t *testing.T) {
	ks := newMockBasicKeystore()
	failPwd := func() ([]byte, error) {
		return nil, fmt.Errorf("terminal closed")
	}
	code := credentialAdd(ks, []string{"-u", "admin"}, failPwd)
	assert.Equal(t, 1, code)
	// Nothing should have been stored
	_, err := ks.get("*")
	assert.Error(t, err)
}

func TestCredentialListStoreListError(t *testing.T) {
	ks := &errorKeystore{
		mockBasicKeystore: newMockBasicKeystore(),
		listErr:           fmt.Errorf("keychain unavailable"),
	}
	var buf bytes.Buffer
	code := credentialList(ks, &buf)
	assert.Equal(t, 1, code)
}

func TestCredentialListSkipsGetErrors(t *testing.T) {
	ks := &errorKeystore{
		mockBasicKeystore: newMockBasicKeystore(),
		getErr:            fmt.Errorf("access denied"),
	}
	_ = ks.mockBasicKeystore.set("proxy.com", "admin:pass")
	var buf bytes.Buffer
	code := credentialList(ks, &buf)
	assert.Equal(t, 0, code)
	output := buf.String()
	// Header is printed but no entries (get fails for each)
	assert.Contains(t, output, "PROXY")
	assert.NotContains(t, output, "admin")
}

func TestCredentialAddOverwritesExisting(t *testing.T) {
	ks := newMockBasicKeystore()
	credentialAdd(ks, []string{"-u", "first", "proxy.com"}, fakePwd("pass1"))
	credentialAdd(ks, []string{"-u", "second", "proxy.com"}, fakePwd("pass2"))
	secret, err := ks.get("proxy.com")
	require.NoError(t, err)
	assert.Equal(t, "second:pass2", secret)
}

func TestCredentialAddEmptyPassword(t *testing.T) {
	ks := newMockBasicKeystore()
	code := credentialAdd(ks, []string{"-u", "user"}, fakePwd(""))
	assert.Equal(t, 0, code)
	secret, err := ks.get("*")
	require.NoError(t, err)
	assert.Equal(t, "user:", secret)
}
