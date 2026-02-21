package profile

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/golangsnmp/mibsh/internal/snmp"
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

// Profile returns an snmp.Profile with the connection parameters from this device.
func (p Device) Profile() snmp.Profile {
	return snmp.Profile{
		Target:        p.Target,
		Community:     p.Community,
		Version:       p.Version,
		SecurityLevel: p.SecurityLevel,
		Username:      p.Username,
		AuthProto:     p.AuthProto,
		AuthPass:      p.AuthPass,
		PrivProto:     p.PrivProto,
		PrivPass:      p.PrivPass,
	}
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
	profiles []Device
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

// Devices returns all saved device profiles.
func (s *Store) Devices() []Device {
	return s.profiles
}

func (s *Store) Load() error {
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
		s.profiles[i].Version = NormalizeVersion(s.profiles[i].Version)
	}
	return nil
}

func (s *Store) Save() error {
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

// Upsert adds or updates a profile by name.
func (s *Store) Upsert(p Device) {
	if i := slices.IndexFunc(s.profiles, func(e Device) bool {
		return e.Name == p.Name
	}); i >= 0 {
		s.profiles[i] = p
		return
	}
	s.profiles = append(s.profiles, p)
}

func (s *Store) Remove(name string) {
	s.profiles = slices.DeleteFunc(s.profiles, func(p Device) bool {
		return p.Name == name
	})
}
