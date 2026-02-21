package snmp

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/gosnmp/gosnmp"
)

// Session holds SNMP connection state.
type Session struct {
	client    *gosnmp.GoSNMP
	Target    string
	Version   string
	connected bool
}

// IsConnected reports whether the session is usable for SNMP operations.
// It is safe to call on a nil receiver.
func (s *Session) IsConnected() bool {
	return s != nil && s.connected && s.client != nil
}

// ConnectMsg is sent when a connection attempt completes.
type ConnectMsg struct {
	Session *Session
	Profile Profile // echo back for profile saving
	Err     error
}

// DisconnectMsg is sent when disconnection completes.
type DisconnectMsg struct{}

// Profile holds connection parameters.
type Profile struct {
	Target    string // host or host:port
	Community string
	Version   string // "1", "2c", "3"

	// SNMPv3 USM fields
	SecurityLevel string // "noAuthNoPriv", "authNoPriv", "authPriv"
	Username      string
	AuthProto     string // "MD5", "SHA", "SHA224", "SHA256", "SHA384", "SHA512"
	AuthPass      string
	PrivProto     string // "DES", "AES", "AES192", "AES256"
	PrivPass      string
}

// ParseVersion converts a version string to the gosnmp version constant.
func ParseVersion(s string) (gosnmp.SnmpVersion, error) {
	switch strings.ToLower(s) {
	case "1", "v1":
		return gosnmp.Version1, nil
	case "2c", "v2c", "2":
		return gosnmp.Version2c, nil
	case "3", "v3":
		return gosnmp.Version3, nil
	default:
		return 0, fmt.Errorf("unknown SNMP version: %s", s)
	}
}

func parseTarget(s string) (string, uint16) {
	host := s
	port := uint16(161)

	// Handle host:port
	if h, p, err := net.SplitHostPort(s); err == nil {
		host = h
		if n, err := strconv.ParseUint(p, 10, 16); err == nil {
			port = uint16(n)
		}
	}
	return host, port
}

func parseSecurityLevel(s string) gosnmp.SnmpV3MsgFlags {
	switch strings.ToLower(s) {
	case "authpriv":
		return gosnmp.AuthPriv
	case "authnopriv":
		return gosnmp.AuthNoPriv
	default:
		return gosnmp.NoAuthNoPriv
	}
}

func parseAuthProto(s string) gosnmp.SnmpV3AuthProtocol {
	switch strings.ToUpper(s) {
	case "MD5":
		return gosnmp.MD5
	case "SHA":
		return gosnmp.SHA
	case "SHA224":
		return gosnmp.SHA224
	case "SHA256":
		return gosnmp.SHA256
	case "SHA384":
		return gosnmp.SHA384
	case "SHA512":
		return gosnmp.SHA512
	default:
		return gosnmp.NoAuth
	}
}

func parsePrivProto(s string) gosnmp.SnmpV3PrivProtocol {
	switch strings.ToUpper(s) {
	case "DES":
		return gosnmp.DES
	case "AES":
		return gosnmp.AES
	case "AES192":
		return gosnmp.AES192
	case "AES256":
		return gosnmp.AES256
	default:
		return gosnmp.NoPriv
	}
}

// ConnectCmd returns a tea.Cmd that connects to an SNMP device.
func ConnectCmd(p Profile) tea.Cmd {
	return func() tea.Msg {
		ver, err := ParseVersion(p.Version)
		if err != nil {
			return ConnectMsg{Err: err}
		}

		host, port := parseTarget(p.Target)

		client := &gosnmp.GoSNMP{
			Target:  host,
			Port:    port,
			Version: ver,
			Timeout: gosnmp.Default.Timeout,
			Retries: gosnmp.Default.Retries,
		}

		if ver == gosnmp.Version3 {
			client.SecurityModel = gosnmp.UserSecurityModel
			client.MsgFlags = parseSecurityLevel(p.SecurityLevel)
			client.SecurityParameters = &gosnmp.UsmSecurityParameters{
				UserName:                 p.Username,
				AuthenticationProtocol:   parseAuthProto(p.AuthProto),
				AuthenticationPassphrase: p.AuthPass,
				PrivacyProtocol:          parsePrivProto(p.PrivProto),
				PrivacyPassphrase:        p.PrivPass,
			}
		} else {
			client.Community = p.Community
		}

		if err := client.Connect(); err != nil {
			return ConnectMsg{Err: err}
		}

		sess := &Session{
			client:    client,
			Target:    p.Target,
			Version:   p.Version,
			connected: true,
		}
		return ConnectMsg{Session: sess, Profile: p}
	}
}

// DisconnectCmd returns a tea.Cmd that disconnects the session.
func DisconnectCmd(sess *Session) tea.Cmd {
	return func() tea.Msg {
		if sess != nil && sess.client != nil && sess.client.Conn != nil {
			_ = sess.client.Conn.Close()
		}
		return DisconnectMsg{}
	}
}
