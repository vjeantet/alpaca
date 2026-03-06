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
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

// mockBasicKeystore is an in-memory implementation of basicKeystore for tests.
type mockBasicKeystore struct {
	entries map[string]string
}

func newMockBasicKeystore() *mockBasicKeystore {
	return &mockBasicKeystore{entries: make(map[string]string)}
}

func (m *mockBasicKeystore) get(account string) (string, error) {
	v, ok := m.entries[account]
	if !ok {
		return "", fmt.Errorf("not found: %s", account)
	}
	return v, nil
}

func (m *mockBasicKeystore) set(account, secret string) error {
	m.entries[account] = secret
	return nil
}

func (m *mockBasicKeystore) delete(account string) error {
	if _, ok := m.entries[account]; !ok {
		return fmt.Errorf("not found: %s", account)
	}
	delete(m.entries, account)
	return nil
}

func (m *mockBasicKeystore) list() ([]string, error) {
	var accounts []string
	for k := range m.entries {
		accounts = append(accounts, k)
	}
	return accounts, nil
}

// countingKeystore wraps mockBasicKeystore and counts get/list calls.
type countingKeystore struct {
	*mockBasicKeystore
	getCalls  int
	listCalls int
	mu        sync.Mutex
}

func newCountingKeystore() *countingKeystore {
	return &countingKeystore{mockBasicKeystore: newMockBasicKeystore()}
}

func (c *countingKeystore) get(account string) (string, error) {
	c.mu.Lock()
	c.getCalls++
	c.mu.Unlock()
	return c.mockBasicKeystore.get(account)
}

func (c *countingKeystore) list() ([]string, error) {
	c.mu.Lock()
	c.listCalls++
	c.mu.Unlock()
	return c.mockBasicKeystore.list()
}

// errorKeystore wraps mockBasicKeystore with injectable errors.
type errorKeystore struct {
	*mockBasicKeystore
	listErr error
	setErr  error
	getErr  error
}

func (e *errorKeystore) get(account string) (string, error) {
	if e.getErr != nil {
		return "", e.getErr
	}
	return e.mockBasicKeystore.get(account)
}

func (e *errorKeystore) list() ([]string, error) {
	if e.listErr != nil {
		return nil, e.listErr
	}
	return e.mockBasicKeystore.list()
}

func (e *errorKeystore) set(account, secret string) error {
	if e.setErr != nil {
		return e.setErr
	}
	return e.mockBasicKeystore.set(account, secret)
}

// --- Resolve logic tests ---

func TestResolveExactMatch(t *testing.T) {
	ks := newMockBasicKeystore()
	_ = ks.set("proxy.corp.com", "admin:secret")
	_ = ks.set("other.corp.com", "user:pass")
	store := &basicCredentialStore{store: ks}
	assert.Equal(t, "admin:secret", store.resolve("proxy.corp.com"))
	assert.Equal(t, "user:pass", store.resolve("other.corp.com"))
}

func TestResolveGlobMatch(t *testing.T) {
	ks := newMockBasicKeystore()
	_ = ks.set("*.corp.com", "glob:pass")
	store := &basicCredentialStore{store: ks}
	assert.Equal(t, "glob:pass", store.resolve("proxy.corp.com"))
	assert.Equal(t, "glob:pass", store.resolve("other.corp.com"))
}

func TestResolveGlobMostSpecificWins(t *testing.T) {
	ks := newMockBasicKeystore()
	_ = ks.set("*.com", "broad:pass")
	_ = ks.set("*.corp.com", "specific:pass")
	store := &basicCredentialStore{store: ks}
	assert.Equal(t, "specific:pass", store.resolve("proxy.corp.com"))
}

func TestResolveFlagOverDefault(t *testing.T) {
	ks := newMockBasicKeystore()
	_ = ks.set("*", "default:pass")
	store := &basicCredentialStore{flagCreds: "flag:pass", store: ks}
	assert.Equal(t, "flag:pass", store.resolve("unknown.host"))
}

func TestResolveDefaultKeychain(t *testing.T) {
	ks := newMockBasicKeystore()
	_ = ks.set("*", "default:pass")
	store := &basicCredentialStore{store: ks}
	assert.Equal(t, "default:pass", store.resolve("unknown.host"))
}

func TestResolveNoCredential(t *testing.T) {
	ks := newMockBasicKeystore()
	store := &basicCredentialStore{store: ks}
	assert.Equal(t, "", store.resolve("unknown.host"))
}

func TestResolveExactOverGlob(t *testing.T) {
	ks := newMockBasicKeystore()
	_ = ks.set("proxy.corp.com", "exact:pass")
	_ = ks.set("*.corp.com", "glob:pass")
	store := &basicCredentialStore{store: ks}
	assert.Equal(t, "exact:pass", store.resolve("proxy.corp.com"))
}

func TestResolveNilStore(t *testing.T) {
	store := &basicCredentialStore{flagCreds: "flag:pass"}
	assert.Equal(t, "flag:pass", store.resolve("any.host"))
}

func TestResolveEmptyProxyHost(t *testing.T) {
	ks := newMockBasicKeystore()
	_ = ks.set("proxy.corp.com", "exact:pass")
	_ = ks.set("*", "default:pass")
	store := &basicCredentialStore{store: ks}
	assert.Equal(t, "default:pass", store.resolve(""))
}

func TestResolveFlagWithNoKeychainEntries(t *testing.T) {
	ks := newMockBasicKeystore()
	store := &basicCredentialStore{flagCreds: "flag:pass", store: ks}
	assert.Equal(t, "flag:pass", store.resolve("any.host"))
}

// --- Cache behavior tests ---

func TestResolveCacheHit(t *testing.T) {
	ks := newCountingKeystore()
	_ = ks.set("proxy.corp.com", "admin:secret")
	store := &basicCredentialStore{store: ks}

	assert.Equal(t, "admin:secret", store.resolve("proxy.corp.com"))
	getAfterFirst := ks.getCalls
	listAfterFirst := ks.listCalls

	// Second call for the same host should hit cache
	assert.Equal(t, "admin:secret", store.resolve("proxy.corp.com"))
	assert.Equal(t, getAfterFirst, ks.getCalls, "get() should not be called again")
	assert.Equal(t, listAfterFirst, ks.listCalls, "list() should not be called again")
}

func TestResolveCachePerHost(t *testing.T) {
	ks := newCountingKeystore()
	_ = ks.set("proxy-a.com", "a:pass")
	_ = ks.set("proxy-b.com", "b:pass")
	store := &basicCredentialStore{store: ks}

	assert.Equal(t, "a:pass", store.resolve("proxy-a.com"))
	assert.Equal(t, "b:pass", store.resolve("proxy-b.com"))
	getAfterBoth := ks.getCalls

	// Both should now be cached
	assert.Equal(t, "a:pass", store.resolve("proxy-a.com"))
	assert.Equal(t, "b:pass", store.resolve("proxy-b.com"))
	assert.Equal(t, getAfterBoth, ks.getCalls, "no additional get() for cached hosts")
}

func TestResolveListCalledOnce(t *testing.T) {
	ks := newCountingKeystore()
	_ = ks.set("a.com", "a:pass")
	_ = ks.set("b.com", "b:pass")
	store := &basicCredentialStore{store: ks}

	store.resolve("a.com")
	store.resolve("b.com")
	store.resolve("unknown.com")
	assert.Equal(t, 1, ks.listCalls, "list() should be called exactly once across all resolves")
}

func TestResolveConcurrent(t *testing.T) {
	ks := newMockBasicKeystore()
	_ = ks.set("proxy.corp.com", "admin:secret")
	_ = ks.set("*.corp.com", "glob:pass")
	_ = ks.set("*", "default:pass")
	store := &basicCredentialStore{store: ks}

	var wg sync.WaitGroup
	hosts := []string{"proxy.corp.com", "other.corp.com", "external.com", ""}
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(host string) {
			defer wg.Done()
			store.resolve(host)
		}(hosts[i%len(hosts)])
	}
	wg.Wait()

	assert.Equal(t, "admin:secret", store.resolve("proxy.corp.com"))
	assert.Equal(t, "glob:pass", store.resolve("other.corp.com"))
	assert.Equal(t, "default:pass", store.resolve("external.com"))
	assert.Equal(t, "default:pass", store.resolve(""))
}

func TestResolveIgnoresKeychainChangesAfterLoad(t *testing.T) {
	ks := newMockBasicKeystore()
	_ = ks.set("proxy.corp.com", "original:pass")
	store := &basicCredentialStore{store: ks}

	assert.Equal(t, "original:pass", store.resolve("proxy.corp.com"))

	// Simulate another process modifying the keychain
	_ = ks.set("proxy.corp.com", "updated:pass")
	_ = ks.set("new.proxy.com", "new:pass")

	// Cached value returned, keychain mutation ignored
	assert.Equal(t, "original:pass", store.resolve("proxy.corp.com"))
	// New entry not visible (load() already classified accounts)
	assert.Equal(t, "", store.resolve("new.proxy.com"))
}

// --- Error path tests ---

func TestResolveListError(t *testing.T) {
	ks := &errorKeystore{
		mockBasicKeystore: newMockBasicKeystore(),
		listErr:           fmt.Errorf("keychain locked"),
	}
	store := &basicCredentialStore{flagCreds: "flag:pass", store: ks}
	assert.Equal(t, "flag:pass", store.resolve("any.host"))
}

func TestResolveListErrorNoFlag(t *testing.T) {
	ks := &errorKeystore{
		mockBasicKeystore: newMockBasicKeystore(),
		listErr:           fmt.Errorf("keychain locked"),
	}
	store := &basicCredentialStore{store: ks}
	assert.Equal(t, "", store.resolve("any.host"))
}

func TestResolveInvalidGlobSkipped(t *testing.T) {
	ks := newMockBasicKeystore()
	_ = ks.set("[invalid", "bad:pass")
	_ = ks.set("proxy.corp.com", "good:pass")
	store := &basicCredentialStore{store: ks}
	assert.Equal(t, "good:pass", store.resolve("proxy.corp.com"))
	// Invalid glob never matches anything
	assert.Equal(t, "", store.resolve("other.com"))
}

func TestResolveGlobNoMatchFallsToDefault(t *testing.T) {
	ks := newMockBasicKeystore()
	_ = ks.set("*.corp.com", "glob:pass")
	_ = ks.set("*", "default:pass")
	store := &basicCredentialStore{store: ks}
	// Glob doesn't match external.com, falls to default
	assert.Equal(t, "default:pass", store.resolve("external.com"))
}

func TestResolveMultipleGlobsSameLengthNoError(t *testing.T) {
	ks := newMockBasicKeystore()
	_ = ks.set("*.aaa.com", "aaa:pass")
	_ = ks.set("*.bbb.com", "bbb:pass")
	store := &basicCredentialStore{store: ks}
	// Each glob only matches its own domain
	assert.Equal(t, "aaa:pass", store.resolve("proxy.aaa.com"))
	assert.Equal(t, "bbb:pass", store.resolve("proxy.bbb.com"))
}
