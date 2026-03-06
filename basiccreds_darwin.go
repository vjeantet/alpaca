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

	"github.com/keybase/go-keychain"
)

const basicKeychainService = "alpaca-basic"

type darwinBasicKeystore struct{}

func newBasicKeystore() basicKeystore {
	return &darwinBasicKeystore{}
}

func (d *darwinBasicKeystore) get(account string) (string, error) {
	query := keychain.NewItem()
	query.SetSecClass(keychain.SecClassGenericPassword)
	query.SetService(basicKeychainService)
	query.SetAccount(account)
	query.SetMatchLimit(keychain.MatchLimitOne)
	query.SetReturnData(true)
	results, err := keychain.QueryItem(query)
	if err != nil {
		return "", fmt.Errorf("keychain query error: %w", err)
	}
	if len(results) == 0 {
		return "", fmt.Errorf("no keychain entry for %q", account)
	}
	return string(results[0].Data), nil
}

func (d *darwinBasicKeystore) set(account, secret string) error {
	item := keychain.NewItem()
	item.SetSecClass(keychain.SecClassGenericPassword)
	item.SetService(basicKeychainService)
	item.SetAccount(account)
	item.SetData([]byte(secret))
	item.SetSynchronizable(keychain.SynchronizableNo)
	item.SetAccessible(keychain.AccessibleWhenUnlocked)
	err := keychain.AddItem(item)
	if err == keychain.ErrorDuplicateItem {
		// Update existing item
		query := keychain.NewItem()
		query.SetSecClass(keychain.SecClassGenericPassword)
		query.SetService(basicKeychainService)
		query.SetAccount(account)
		update := keychain.NewItem()
		update.SetData([]byte(secret))
		return keychain.UpdateItem(query, update)
	}
	return err
}

func (d *darwinBasicKeystore) delete(account string) error {
	item := keychain.NewItem()
	item.SetSecClass(keychain.SecClassGenericPassword)
	item.SetService(basicKeychainService)
	item.SetAccount(account)
	return keychain.DeleteItem(item)
}

func (d *darwinBasicKeystore) list() ([]string, error) {
	query := keychain.NewItem()
	query.SetSecClass(keychain.SecClassGenericPassword)
	query.SetService(basicKeychainService)
	query.SetMatchLimit(keychain.MatchLimitAll)
	query.SetReturnAttributes(true)
	results, err := keychain.QueryItem(query)
	if err == keychain.ErrorItemNotFound || len(results) == 0 {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("keychain list error: %w", err)
	}
	accounts := make([]string, 0, len(results))
	for _, r := range results {
		accounts = append(accounts, r.Account)
	}
	return accounts, nil
}
