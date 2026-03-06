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

//go:build !darwin

package main

import (
	"fmt"
	"strings"

	ring "github.com/zalando/go-keyring"
)

const basicKeyringService = "alpaca-basic"
const basicKeyringIndex = "__index__"

type otherBasicKeystore struct{}

func newBasicKeystore() basicKeystore {
	return &otherBasicKeystore{}
}

func (o *otherBasicKeystore) get(account string) (string, error) {
	secret, err := ring.Get(basicKeyringService, account)
	if err != nil {
		return "", fmt.Errorf("keyring get error: %w", err)
	}
	return secret, nil
}

func (o *otherBasicKeystore) set(account, secret string) error {
	if err := ring.Set(basicKeyringService, account, secret); err != nil {
		return err
	}
	return o.addToIndex(account)
}

func (o *otherBasicKeystore) delete(account string) error {
	if err := ring.Delete(basicKeyringService, account); err != nil {
		return err
	}
	return o.removeFromIndex(account)
}

func (o *otherBasicKeystore) list() ([]string, error) {
	indexStr, err := ring.Get(basicKeyringService, basicKeyringIndex)
	if err != nil {
		return nil, nil // no index means no entries
	}
	if indexStr == "" {
		return nil, nil
	}
	entries := strings.Split(indexStr, "\n")
	var result []string
	for _, e := range entries {
		if e != "" {
			result = append(result, e)
		}
	}
	return result, nil
}

func (o *otherBasicKeystore) addToIndex(account string) error {
	accounts, _ := o.list()
	for _, a := range accounts {
		if a == account {
			return nil // already indexed
		}
	}
	accounts = append(accounts, account)
	return ring.Set(basicKeyringService, basicKeyringIndex,
		strings.Join(accounts, "\n"))
}

func (o *otherBasicKeystore) removeFromIndex(account string) error {
	accounts, _ := o.list()
	var filtered []string
	for _, a := range accounts {
		if a != account {
			filtered = append(filtered, a)
		}
	}
	if len(filtered) == 0 {
		_ = ring.Delete(basicKeyringService, basicKeyringIndex)
		return nil
	}
	return ring.Set(basicKeyringService, basicKeyringIndex,
		strings.Join(filtered, "\n"))
}
