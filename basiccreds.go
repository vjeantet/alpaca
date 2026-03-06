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
	"log"
	"sort"
	"strings"
	"sync"

	"github.com/gobwas/glob"
)

// defaultProxyPattern is the keychain account name reserved for the
// fallback credential that applies when no specific proxy match is found.
const defaultProxyPattern = "*"

// basicKeystore abstracts access to the system keychain for basic auth
// credentials. Each entry is keyed by proxy pattern (account) and stores
// a "login:password" string as the secret. The account defaultProxyPattern
// is reserved for the fallback credential.
type basicKeystore interface {
	get(account string) (string, error)
	set(account, secret string) error
	delete(account string) error
	list() ([]string, error) // returns all account names
}

type compiledGlob struct {
	pattern string
	g       glob.Glob
}

// basicCredentialStore resolves which basic auth credential to use for a
// given proxy host. It checks the keychain first, then falls back to the
// -b flag value. Account data and compiled globs are loaded once on first
// use and resolved credentials are cached per proxy host.
type basicCredentialStore struct {
	flagCreds string // value of the -b flag
	store     basicKeystore

	initOnce sync.Once
	exact    []string       // non-glob, non-default accounts
	globs    []compiledGlob // sorted by pattern length descending

	mu    sync.RWMutex
	cache map[string]string // proxyHost -> "login:password"
}

func (s *basicCredentialStore) load() {
	s.cache = make(map[string]string)
	if s.store == nil {
		return
	}
	accounts, err := s.store.list()
	if err != nil {
		log.Printf("Error listing basic credentials: %v", err)
		return
	}
	for _, acct := range accounts {
		if acct == defaultProxyPattern {
			continue
		}
		if strings.ContainsAny(acct, "*?[") {
			g, err := glob.Compile(acct)
			if err != nil {
				continue
			}
			s.globs = append(s.globs, compiledGlob{pattern: acct, g: g})
		} else {
			s.exact = append(s.exact, acct)
		}
	}
	// Most specific glob first (longest pattern).
	sort.Slice(s.globs, func(i, j int) bool {
		return len(s.globs[i].pattern) > len(s.globs[j].pattern)
	})
}

// resolve returns the "login:password" credential to use for the given
// proxy host. Resolution order:
//  1. Exact hostname match in keychain
//  2. Glob pattern match in keychain (most specific wins)
//  3. -b flag value
//  4. Default keychain entry (account == defaultProxyPattern)
//  5. Empty string (no credential)
//
// Results are cached per proxy host for the lifetime of the process.
func (s *basicCredentialStore) resolve(proxyHost string) string {
	s.initOnce.Do(s.load)

	if s.store == nil {
		return s.flagCreds
	}

	s.mu.RLock()
	if cred, ok := s.cache[proxyHost]; ok {
		s.mu.RUnlock()
		return cred
	}
	s.mu.RUnlock()

	cred := s.doResolve(proxyHost)

	s.mu.Lock()
	s.cache[proxyHost] = cred
	s.mu.Unlock()

	return cred
}

func (s *basicCredentialStore) doResolve(proxyHost string) string {
	if proxyHost != "" {
		// 1. Exact hostname match
		for _, acct := range s.exact {
			if acct == proxyHost {
				if creds, err := s.store.get(acct); err == nil {
					return creds
				}
			}
		}
		// 2. Glob pattern match (already sorted most-specific-first)
		for _, cg := range s.globs {
			if cg.g.Match(proxyHost) {
				if creds, err := s.store.get(cg.pattern); err == nil {
					return creds
				}
			}
		}
	}

	// 3. -b flag
	if s.flagCreds != "" {
		return s.flagCreds
	}

	// 4. Default keychain entry
	if creds, err := s.store.get(defaultProxyPattern); err == nil && creds != "" {
		return creds
	}

	// 5. No credential
	return ""
}
