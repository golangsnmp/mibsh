package profile

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Device stores SNMP connection parameters for a saved device.
type Device struct {
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

func (p Device) Summary() string {
	s := p.Target + ", v" + p.Version
	if p.IsV3() {
		s += ", " + p.Username
	}
	return s
}

func (p Device) IsV3() bool {
	return p.Version == "3"
}

// NormalizeVersion strips a leading "v" prefix and lowercases the version string
// so that comparisons like IsV3() only need to check the canonical form.
func NormalizeVersion(v string) string {
	v = strings.ToLower(v)
	v = strings.TrimPrefix(v, "v")
	return v
}

// Store manages loading and saving device profiles to disk.
type Store struct {
	path     string
	Profiles []Device
}

func NewStore() *Store {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return &Store{
		path: filepath.Join(dir, "mibsh", "profiles.json"),
	}
}

func (s *Store) Load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	if err := json.Unmarshal(data, &s.Profiles); err != nil {
		return err
	}
	for i := range s.Profiles {
		s.Profiles[i].Version = NormalizeVersion(s.Profiles[i].Version)
	}
	return nil
}

func (s *Store) Save() error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.Profiles, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}

// Upsert adds or updates a profile by name.
func (s *Store) Upsert(p Device) {
	if i := slices.IndexFunc(s.Profiles, func(e Device) bool {
		return e.Name == p.Name
	}); i >= 0 {
		s.Profiles[i] = p
		return
	}
	s.Profiles = append(s.Profiles, p)
}

func (s *Store) Remove(name string) {
	s.Profiles = slices.DeleteFunc(s.Profiles, func(p Device) bool {
		return p.Name == name
	})
}
