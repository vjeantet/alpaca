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
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

func patternLabel(pattern string) string {
	if pattern == defaultProxyPattern {
		return "(default)"
	}
	return pattern
}

func runCredentialCommand(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: alpaca credential <add|remove|list>")
		return 1
	}
	store := newBasicKeystore()
	readPwd := func() ([]byte, error) {
		return term.ReadPassword(int(os.Stdin.Fd()))
	}
	switch args[0] {
	case "add":
		return credentialAdd(store, args[1:], readPwd)
	case "remove":
		return credentialRemove(store, args[1:])
	case "list":
		return credentialList(store, os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "Unknown credential subcommand: %s\n", args[0])
		fmt.Fprintln(os.Stderr, "Usage: alpaca credential <add|remove|list>")
		return 1
	}
}

func credentialAdd(store basicKeystore, args []string, readPwd func() ([]byte, error)) int {
	fs := flag.NewFlagSet("credential add", flag.ContinueOnError)
	login := fs.String("u", "", "login (username)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *login == "" {
		fmt.Fprintln(os.Stderr, "Error: -u <login> is required")
		return 1
	}
	if strings.Contains(*login, ":") {
		fmt.Fprintln(os.Stderr, "Error: login must not contain ':'")
		return 1
	}

	proxyPattern := defaultProxyPattern
	if fs.NArg() > 0 {
		proxyPattern = fs.Arg(0)
	}

	fmt.Fprintf(os.Stderr, "Password (for %s): ", *login)
	pwd, err := readPwd()
	fmt.Fprintln(os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading password: %v\n", err)
		return 1
	}

	secret := *login + ":" + string(pwd)
	if err := store.set(proxyPattern, secret); err != nil {
		fmt.Fprintf(os.Stderr, "Error storing credential: %v\n", err)
		return 1
	}

	fmt.Fprintf(os.Stderr, "Credential stored for %s\n", patternLabel(proxyPattern))
	return 0
}

func credentialRemove(store basicKeystore, args []string) int {
	proxyPattern := defaultProxyPattern
	if len(args) > 0 {
		proxyPattern = args[0]
	}
	if err := store.delete(proxyPattern); err != nil {
		fmt.Fprintf(os.Stderr, "Error removing credential: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "Credential removed for %s\n", patternLabel(proxyPattern))
	return 0
}

func credentialList(store basicKeystore, out io.Writer) int {
	accounts, err := store.list()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing credentials: %v\n", err)
		return 1
	}
	if len(accounts) == 0 {
		fmt.Fprintln(out, "No credentials stored.")
		return 0
	}
	fmt.Fprintf(out, "%-30s %s\n", "PROXY", "LOGIN")
	for _, acct := range accounts {
		secret, err := store.get(acct)
		if err != nil {
			continue
		}
		login := secret
		if idx := strings.IndexByte(secret, ':'); idx >= 0 {
			login = secret[:idx]
		}
		fmt.Fprintf(out, "%-30s %s\n", patternLabel(acct), login)
	}
	return 0
}
