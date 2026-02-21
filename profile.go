package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// deviceProfile stores SNMP connection parameters for a saved device.
type deviceProfile struct {
	Name      string `json:"name"`
	Target    string `json:"target"`
	Community string `json:"community,omitempty"`
	Version   string `json:"version"`

	// SNMPv3 USM fields
	SecurityLevel string `json:"security_level,omitempty"` // noAuthNoPriv, authNoPriv, authPriv
	Username      string `json:"username,omitempty"`
	AuthProto     string `json:"auth_proto,omitempty"` // MD5, SHA, SHA224, SHA256, SHA384, SHA512
	AuthPass      string `json:"auth_pass,omitempty"`
	PrivProto     string `json:"priv_proto,omitempty"` // DES, AES, AES192, AES256
	PrivPass      string `json:"priv_pass,omitempty"`
}

func (p deviceProfile) toSnmpProfile() snmpProfile {
	return snmpProfile{
		target:        p.Target,
		community:     p.Community,
		version:       p.Version,
		securityLevel: p.SecurityLevel,
		username:      p.Username,
		authProto:     p.AuthProto,
		authPass:      p.AuthPass,
		privProto:     p.PrivProto,
		privPass:      p.PrivPass,
	}
}

func (p deviceProfile) summary() string {
	s := p.Target + ", v" + p.Version
	if p.isV3() {
		s += ", " + p.Username
	}
	return s
}

func (p deviceProfile) isV3() bool {
	return p.Version == "3"
}

// normalizeVersion strips a leading "v" prefix and lowercases the version string
// so that comparisons like isV3() only need to check the canonical form.
func normalizeVersion(v string) string {
	v = strings.ToLower(v)
	v = strings.TrimPrefix(v, "v")
	return v
}

// profileStore manages loading and saving device profiles to disk.
type profileStore struct {
	path     string
	profiles []deviceProfile
}

func newProfileStore() *profileStore {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return &profileStore{
		path: filepath.Join(dir, "mibsh", "profiles.json"),
	}
}

func (s *profileStore) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	if err := json.Unmarshal(data, &s.profiles); err != nil {
		return err
	}
	for i := range s.profiles {
		s.profiles[i].Version = normalizeVersion(s.profiles[i].Version)
	}
	return nil
}

func (s *profileStore) save() error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.profiles, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}

// upsert adds or updates a profile by name.
func (s *profileStore) upsert(p deviceProfile) {
	if i := slices.IndexFunc(s.profiles, func(e deviceProfile) bool {
		return e.Name == p.Name
	}); i >= 0 {
		s.profiles[i] = p
		return
	}
	s.profiles = append(s.profiles, p)
}

func (s *profileStore) remove(name string) {
	s.profiles = slices.DeleteFunc(s.profiles, func(p deviceProfile) bool {
		return p.Name == name
	})
}
