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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig_FileNotFound(t *testing.T) {
	cfg, err := loadConfig("/nonexistent/path/config.yaml")
	require.NoError(t, err)
	assert.Equal(t, config{}, cfg)
}

func TestLoadConfig_ValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
listen:
  - 127.0.0.1
  - "::1"
port: 9090
pac-url: http://example.com/proxy.pac
domain: CORP
username: jdoe
kerberos: true
kerberos-wait: 60
quiet: true
log-level: debug
log-format: json
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	cfg, err := loadConfig(path)
	require.NoError(t, err)
	assert.Equal(t, []string{"127.0.0.1", "::1"}, cfg.Listen)
	assert.Equal(t, 9090, cfg.Port)
	assert.Equal(t, "http://example.com/proxy.pac", cfg.PACUrl)
	assert.Equal(t, "CORP", cfg.Domain)
	assert.Equal(t, "jdoe", cfg.Username)
	assert.True(t, cfg.Kerberos)
	assert.Equal(t, 60, cfg.KerberosWait)
	assert.True(t, cfg.Quiet)
	assert.Equal(t, "debug", cfg.LogLevel)
	assert.Equal(t, "json", cfg.LogFormat)
}

func TestLoadConfig_PasswordRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
port: 3128
password: secret
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	_, err := loadConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "password")
	assert.Contains(t, err.Error(), "not allowed")
}

func TestLoadConfig_BasicCredsRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
port: 3128
basic-credentials: user:pass
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	_, err := loadConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "basic-credentials")
	assert.Contains(t, err.Error(), "not allowed")
}

func TestLoadConfig_MalformedYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `port: [invalid`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	_, err := loadConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing config file")
}

func TestLoadConfig_PartialFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
port: 8080
domain: MYDOM
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	cfg, err := loadConfig(path)
	require.NoError(t, err)
	assert.Equal(t, 8080, cfg.Port)
	assert.Equal(t, "MYDOM", cfg.Domain)
	assert.Empty(t, cfg.Listen)
	assert.Empty(t, cfg.PACUrl)
	assert.Empty(t, cfg.Username)
	assert.False(t, cfg.Kerberos)
	assert.Equal(t, 0, cfg.KerberosWait)
	assert.False(t, cfg.Quiet)
	assert.Empty(t, cfg.LogLevel)
	assert.Empty(t, cfg.LogFormat)
}

func TestApplyConfig_CLIWins(t *testing.T) {
	cfg := config{Port: 9090, Domain: "FROMFILE"}
	explicit := map[string]bool{"p": true}

	port := 3128
	domain := ""
	hosts := stringArrayFlag{}
	pacurl := ""
	username := ""
	kerberos := false
	kerberosWait := 30
	quiet := false
	logLevel := "info"
	logFormat := "text"

	applyConfig(cfg, explicit, &hosts, &port, &pacurl, &domain, &username,
		&kerberos, &kerberosWait, &quiet, &logLevel, &logFormat)

	assert.Equal(t, 3128, port, "CLI flag should win over config")
	assert.Equal(t, "FROMFILE", domain, "config should apply when CLI flag not set")
}

func TestApplyConfig_ConfigApplied(t *testing.T) {
	cfg := config{
		Port:         9090,
		PACUrl:       "http://pac.example.com",
		Domain:       "CORP",
		Username:     "alice",
		Kerberos:     true,
		KerberosWait: 45,
		Quiet:        true,
		LogLevel:     "debug",
		LogFormat:    "json",
	}
	explicit := map[string]bool{}

	port := 3128
	pacurl := ""
	domain := ""
	username := ""
	kerberos := false
	kerberosWait := 30
	quiet := false
	logLevel := "info"
	logFormat := "text"
	hosts := stringArrayFlag{}

	applyConfig(cfg, explicit, &hosts, &port, &pacurl, &domain, &username,
		&kerberos, &kerberosWait, &quiet, &logLevel, &logFormat)

	assert.Equal(t, 9090, port)
	assert.Equal(t, "http://pac.example.com", pacurl)
	assert.Equal(t, "CORP", domain)
	assert.Equal(t, "alice", username)
	assert.True(t, kerberos)
	assert.Equal(t, 45, kerberosWait)
	assert.True(t, quiet)
	assert.Equal(t, "debug", logLevel)
	assert.Equal(t, "json", logFormat)
}

func TestApplyConfig_EmptyConfigNoChange(t *testing.T) {
	cfg := config{}
	explicit := map[string]bool{}

	port := 3128
	pacurl := ""
	domain := ""
	username := "me"
	kerberos := false
	kerberosWait := 30
	quiet := false
	logLevel := "info"
	logFormat := "text"
	hosts := stringArrayFlag{"localhost"}

	applyConfig(cfg, explicit, &hosts, &port, &pacurl, &domain, &username,
		&kerberos, &kerberosWait, &quiet, &logLevel, &logFormat)

	assert.Equal(t, 3128, port)
	assert.Equal(t, "me", username)
	assert.Equal(t, stringArrayFlag{"localhost"}, hosts)
}

func TestApplyConfig_ListenFromConfig(t *testing.T) {
	cfg := config{Listen: []string{"0.0.0.0", "::1"}}
	explicit := map[string]bool{}

	hosts := stringArrayFlag{}
	port := 3128
	pacurl := ""
	domain := ""
	username := ""
	kerberos := false
	kerberosWait := 30
	quiet := false
	logLevel := "info"
	logFormat := "text"

	applyConfig(cfg, explicit, &hosts, &port, &pacurl, &domain, &username,
		&kerberos, &kerberosWait, &quiet, &logLevel, &logFormat)

	assert.Equal(t, stringArrayFlag{"0.0.0.0", "::1"}, hosts)
}

func TestDefaultConfigPath(t *testing.T) {
	path, err := defaultConfigPath()
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(path, filepath.Join(".config", "alpaca", "config.yaml")))
}

func TestCreateDefaultConfig_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	created, err := createDefaultConfig(path)
	require.NoError(t, err)
	assert.True(t, created)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, defaultConfigContent, string(data))
}

func TestCreateDefaultConfig_SkipsIfExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	existing := "port: 9090\n"
	require.NoError(t, os.WriteFile(path, []byte(existing), 0o644))

	created, err := createDefaultConfig(path)
	require.NoError(t, err)
	assert.False(t, created)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, existing, string(data), "existing file should not be overwritten")
}

func TestCreateDefaultConfig_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deep", "nested", "config.yaml")

	created, err := createDefaultConfig(path)
	require.NoError(t, err)
	assert.True(t, created)

	info, err := os.Stat(filepath.Join(dir, "deep", "nested"))
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, defaultConfigContent, string(data))
}
