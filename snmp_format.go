package main

import (
	"encoding/hex"
	"fmt"
	"math"
	"net"
	"strings"
	"unicode/utf8"

	"github.com/golangsnmp/gomib/mib"
	"github.com/gosnmp/gosnmp"
)

// formatPDU formats an SNMP PDU value using MIB context when available.
// node and m may be nil for raw formatting.
func formatPDU(pdu gosnmp.SnmpPDU, node *mib.Node, m *mib.Mib) string {
	switch pdu.Type {
	case gosnmp.NoSuchObject:
		return "noSuchObject"
	case gosnmp.NoSuchInstance:
		return "noSuchInstance"
	case gosnmp.EndOfMibView:
		return "endOfMibView"
	case gosnmp.Null:
		return "NULL"
	}

	var obj *mib.Object
	if node != nil {
		obj = node.Object()
	}

	switch pdu.Type {
	case gosnmp.Integer:
		return formatInteger(pdu.Value, obj)
	case gosnmp.OctetString:
		return formatOctetString(pdu.Value, obj)
	case gosnmp.ObjectIdentifier:
		return formatOID(pdu.Value, m)
	case gosnmp.TimeTicks:
		return formatTimeTicks(pdu.Value)
	case gosnmp.IPAddress:
		return fmt.Sprintf("%s", pdu.Value)
	case gosnmp.Counter32:
		return fmt.Sprintf("%d", pdu.Value)
	case gosnmp.Gauge32:
		return fmt.Sprintf("%d", pdu.Value)
	case gosnmp.Counter64:
		return fmt.Sprintf("%d", gosnmp.ToBigInt(pdu.Value).Uint64())
	case gosnmp.Uinteger32:
		return fmt.Sprintf("%d", pdu.Value)
	case gosnmp.Opaque:
		if b, ok := pdu.Value.([]byte); ok {
			return hex.EncodeToString(b)
		}
		return fmt.Sprintf("%v", pdu.Value)
	case gosnmp.OpaqueFloat:
		return fmt.Sprintf("%g", pdu.Value)
	case gosnmp.OpaqueDouble:
		return fmt.Sprintf("%g", pdu.Value)
	default:
		return fmt.Sprintf("%v", pdu.Value)
	}
}

func formatInteger(val any, obj *mib.Object) string {
	v, ok := toInt64(val)
	if !ok {
		return fmt.Sprintf("%v", val)
	}

	// Enum resolution
	if obj != nil {
		for _, nv := range obj.EffectiveEnums() {
			if nv.Value == v {
				return fmt.Sprintf("%s(%d)", nv.Label, v)
			}
		}
	}

	return fmt.Sprintf("%d", v)
}

func formatOctetString(val any, obj *mib.Object) string {
	b, ok := val.([]byte)
	if !ok {
		return fmt.Sprintf("%v", val)
	}

	// BITS resolution
	if obj != nil {
		bits := obj.EffectiveBits()
		if len(bits) > 0 {
			return formatBits(b, bits)
		}
	}

	// Display hint
	if obj != nil {
		hint := obj.EffectiveDisplayHint()
		if hint != "" {
			if s, ok := applyDisplayHint(hint, b); ok {
				return s
			}
		}
	}

	// Valid UTF-8 text (covers ASCII and multibyte)
	if utf8.Valid(b) && isPrintable(b) {
		return strings.ToValidUTF8(string(b), "\ufffd")
	}

	// IP address (4 or 16 bytes)
	if len(b) == 4 || len(b) == 16 {
		return net.IP(b).String()
	}

	// MAC address (6 bytes)
	if len(b) == 6 {
		return fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X", b[0], b[1], b[2], b[3], b[4], b[5])
	}

	return hex.EncodeToString(b)
}

func formatBits(data []byte, bits []mib.NamedValue) string {
	var set []string
	for _, nv := range bits {
		byteIdx := nv.Value / 8
		bitIdx := 7 - (nv.Value % 8)
		if int(byteIdx) < len(data) && data[byteIdx]&(1<<bitIdx) != 0 {
			set = append(set, nv.Label)
		}
	}
	if len(set) == 0 {
		return "(none)"
	}
	return strings.Join(set, ", ")
}

func formatOID(val any, m *mib.Mib) string {
	s, ok := val.(string)
	if !ok {
		return fmt.Sprintf("%v", val)
	}

	if m != nil {
		oid, err := mib.ParseOID(s)
		if err == nil {
			if node := m.LongestPrefixByOID(oid); node != nil && node.Name() != "" {
				suffix := oid[len(node.OID()):]
				if len(suffix) == 0 {
					return node.Name()
				}
				var parts []string
				for _, arc := range suffix {
					parts = append(parts, fmt.Sprintf("%d", arc))
				}
				return node.Name() + "." + strings.Join(parts, ".")
			}
		}
	}

	return s
}

func formatTimeTicks(val any) string {
	ticks, ok := toUint64(val)
	if !ok {
		return fmt.Sprintf("%v", val)
	}

	// TimeTicks are in hundredths of a second
	totalSecs := ticks / 100
	days := totalSecs / 86400
	hours := (totalSecs % 86400) / 3600
	mins := (totalSecs % 3600) / 60
	secs := totalSecs % 60

	if days > 0 {
		return fmt.Sprintf("%d days, %02d:%02d:%02d (%d)", days, hours, mins, secs, ticks)
	}
	return fmt.Sprintf("%02d:%02d:%02d (%d)", hours, mins, secs, ticks)
}

// pduTypeName returns a short display name for the PDU type.
func pduTypeName(t gosnmp.Asn1BER) string {
	switch t {
	case gosnmp.Integer:
		return "INTEGER"
	case gosnmp.OctetString:
		return "STRING"
	case gosnmp.ObjectIdentifier:
		return "OID"
	case gosnmp.IPAddress:
		return "IpAddress"
	case gosnmp.Counter32:
		return "Counter32"
	case gosnmp.Gauge32:
		return "Gauge32"
	case gosnmp.TimeTicks:
		return "TimeTicks"
	case gosnmp.Opaque:
		return "Opaque"
	case gosnmp.Counter64:
		return "Counter64"
	case gosnmp.Uinteger32:
		return "Uinteger32"
	case gosnmp.NoSuchObject:
		return "noSuchObject"
	case gosnmp.NoSuchInstance:
		return "noSuchInstance"
	case gosnmp.EndOfMibView:
		return "endOfMibView"
	case gosnmp.Null:
		return "NULL"
	default:
		return fmt.Sprintf("0x%02x", byte(t))
	}
}

// isPrintable returns true if b looks like displayable text (no control
// characters other than tab/newline/carriage return). Works with UTF-8.
func isPrintable(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	for _, c := range b {
		if c < 32 && c != '\t' && c != '\n' && c != '\r' {
			return false
		}
		if c == 0x7f {
			return false
		}
	}
	return true
}

// formatPDUToResult formats an SNMP PDU into a display-ready snmpResult,
// resolving OID names via the MIB.
func formatPDUToResult(pdu gosnmp.SnmpPDU, m *mib.Mib) snmpResult {
	oid, _ := mib.ParseOID(pdu.Name)
	var node *mib.Node
	name := pdu.Name
	if oid != nil {
		node = m.LongestPrefixByOID(oid)
		if node != nil && node.Name() != "" {
			suffix := oid[len(node.OID()):]
			name = node.Name()
			if len(suffix) > 0 {
				name += "." + mib.OID(suffix).String()
			}
		}
	}
	return snmpResult{
		oid:      pdu.Name,
		name:     name,
		value:    formatPDU(pdu, node, m),
		typeName: pduTypeName(pdu.Type),
	}
}

func toInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int64:
		return n, true
	case int32:
		return int64(n), true
	case uint:
		if uint64(n) > math.MaxInt64 {
			return 0, false
		}
		return int64(n), true
	case uint64:
		if n > math.MaxInt64 {
			return 0, false
		}
		return int64(n), true
	case uint32:
		return int64(n), true
	}
	return 0, false
}

func toUint64(v any) (uint64, bool) {
	switch n := v.(type) {
	case uint:
		return uint64(n), true
	case uint64:
		return n, true
	case uint32:
		return uint64(n), true
	case int:
		if n < 0 {
			return 0, false
		}
		return uint64(n), true
	case int64:
		if n < 0 {
			return 0, false
		}
		return uint64(n), true
	case int32:
		if n < 0 {
			return 0, false
		}
		return uint64(n), true
	}
	return 0, false
}
