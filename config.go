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
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type config struct {
	Listen           []string `yaml:"listen"`
	Port             int      `yaml:"port"`
	PACUrl           string   `yaml:"pac-url"`
	Domain           string   `yaml:"domain"`
	Username         string   `yaml:"username"`
	Kerberos         bool     `yaml:"kerberos"`
	KerberosWait     int      `yaml:"kerberos-wait"`
	Quiet            bool     `yaml:"quiet"`
	JSONLogs         bool     `yaml:"json-logs"`
	Password         string   `yaml:"password"`
	BasicCredentials string   `yaml:"basic-credentials"`
}

func defaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "alpaca", "config.yaml"), nil
}

func loadConfig(path string) (config, error) {
	var cfg config
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parsing config file %s: %w", path, err)
	}
	if cfg.Password != "" {
		return cfg, fmt.Errorf(
			"config file %s: \"password\" is not allowed in config file for security reasons",
			path,
		)
	}
	if cfg.BasicCredentials != "" {
		return cfg, fmt.Errorf(
			"config file %s: \"basic-credentials\" is not allowed in config file for security reasons",
			path,
		)
	}
	return cfg, nil
}

func explicitlySetFlags() map[string]bool {
	set := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) {
		set[f.Name] = true
	})
	return set
}

func applyConfig(
	cfg config,
	explicit map[string]bool,
	hosts *stringArrayFlag,
	port *int,
	pacurl *string,
	domain *string,
	username *string,
	kerberos *bool,
	kerberosWait *int,
	quiet *bool,
	jsonLogs *bool,
) {
	if len(cfg.Listen) > 0 && !explicit["l"] {
		*hosts = cfg.Listen
	}
	if cfg.Port != 0 && !explicit["p"] {
		*port = cfg.Port
	}
	if cfg.PACUrl != "" && !explicit["C"] {
		*pacurl = cfg.PACUrl
	}
	if cfg.Domain != "" && !explicit["d"] {
		*domain = cfg.Domain
	}
	if cfg.Username != "" && !explicit["u"] {
		*username = cfg.Username
	}
	if cfg.Kerberos && !explicit["k"] {
		*kerberos = cfg.Kerberos
	}
	if cfg.KerberosWait != 0 && !explicit["w"] {
		*kerberosWait = cfg.KerberosWait
	}
	if cfg.Quiet && !explicit["q"] {
		*quiet = cfg.Quiet
	}
	if cfg.JSONLogs && !explicit["json-logs"] {
		*jsonLogs = cfg.JSONLogs
	}
}

func logConfigSources(cfg config, explicit map[string]bool, configPath string) {
	type entry struct {
		flagName string
		label    string
		value    any
		active   bool
	}
	entries := []entry{
		{"l", "listen", cfg.Listen, len(cfg.Listen) > 0},
		{"p", "port", cfg.Port, cfg.Port != 0},
		{"C", "pac-url", cfg.PACUrl, cfg.PACUrl != ""},
		{"d", "domain", cfg.Domain, cfg.Domain != ""},
		{"u", "username", cfg.Username, cfg.Username != ""},
		{"k", "kerberos", cfg.Kerberos, cfg.Kerberos},
		{"w", "kerberos-wait", cfg.KerberosWait, cfg.KerberosWait != 0},
		{"q", "quiet", cfg.Quiet, cfg.Quiet},
		{"json-logs", "json-logs", cfg.JSONLogs, cfg.JSONLogs},
	}
	logged := false
	for _, e := range entries {
		if !e.active {
			continue
		}
		if explicit[e.flagName] {
			continue
		}
		if !logged {
			log.Printf("Loaded config from %s", configPath)
			logged = true
		}
		log.Printf("  %s = %v (from config file)", e.label, e.value)
	}
}
