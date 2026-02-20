package main

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/gosnmp/gosnmp"
)

// snmpSession holds SNMP connection state.
type snmpSession struct {
	client    *gosnmp.GoSNMP
	target    string
	version   string
	connected bool
}

// snmpConnectMsg is sent when a connection attempt completes.
type snmpConnectMsg struct {
	session *snmpSession
	profile snmpProfile // echo back for profile saving
	err     error
}

// snmpDisconnectMsg is sent when disconnection completes.
type snmpDisconnectMsg struct{}

// snmpProfile holds connection parameters.
type snmpProfile struct {
	target    string // host or host:port
	community string
	version   string // "1", "2c", "3"

	// SNMPv3 USM fields
	securityLevel string // "noAuthNoPriv", "authNoPriv", "authPriv"
	username      string
	authProto     string // "MD5", "SHA", "SHA224", "SHA256", "SHA384", "SHA512"
	authPass      string
	privProto     string // "DES", "AES", "AES192", "AES256"
	privPass      string
}

func parseVersion(s string) (gosnmp.SnmpVersion, error) {
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

// connectCmd returns a tea.Cmd that connects to an SNMP device.
func connectCmd(p snmpProfile) tea.Cmd {
	return func() tea.Msg {
		ver, err := parseVersion(p.version)
		if err != nil {
			return snmpConnectMsg{err: err}
		}

		host, port := parseTarget(p.target)

		client := &gosnmp.GoSNMP{
			Target:  host,
			Port:    port,
			Version: ver,
			Timeout: gosnmp.Default.Timeout,
			Retries: gosnmp.Default.Retries,
		}

		if ver == gosnmp.Version3 {
			client.SecurityModel = gosnmp.UserSecurityModel
			client.MsgFlags = parseSecurityLevel(p.securityLevel)
			client.SecurityParameters = &gosnmp.UsmSecurityParameters{
				UserName:                 p.username,
				AuthenticationProtocol:   parseAuthProto(p.authProto),
				AuthenticationPassphrase: p.authPass,
				PrivacyProtocol:          parsePrivProto(p.privProto),
				PrivacyPassphrase:        p.privPass,
			}
		} else {
			client.Community = p.community
		}

		if err := client.Connect(); err != nil {
			return snmpConnectMsg{err: err}
		}

		sess := &snmpSession{
			client:    client,
			target:    p.target,
			version:   p.version,
			connected: true,
		}
		return snmpConnectMsg{session: sess, profile: p}
	}
}

// disconnectCmd returns a tea.Cmd that disconnects the session.
func disconnectCmd(sess *snmpSession) tea.Cmd {
	return func() tea.Msg {
		if sess != nil && sess.client != nil && sess.client.Conn != nil {
			_ = sess.client.Conn.Close()
		}
		return snmpDisconnectMsg{}
	}
}
