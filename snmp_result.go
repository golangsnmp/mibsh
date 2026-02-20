package main

import (
	"github.com/golangsnmp/gomib/mib"
)

// opKind identifies the type of SNMP operation that produced a result group.
type opKind int

const (
	opGet opKind = iota
	opGetNext
	opWalk
)

// snmpResult is a single formatted SNMP result.
type snmpResult struct {
	oid      string // dotted OID string
	name     string // resolved name (e.g. "sysDescr.0")
	value    string // formatted value
	typeName string // type label (e.g. "STRING", "INTEGER")
}

// resultGroup is a set of results from a single SNMP operation.
type resultGroup struct {
	op          opKind
	label       string // short description (e.g. "GET sysDescr.0")
	results     []snmpResult
	err         error   // non-nil if the operation failed
	walkRootOID mib.OID // root OID for walk operations (used by tree view)
}

const resultHistoryCapacity = 50

// resultHistory is a ring buffer of result groups.
type resultHistory struct {
	groups []resultGroup
	cursor int // index of the currently viewed group (-1 if empty)
}

func (h *resultHistory) add(g resultGroup) {
	if len(h.groups) >= resultHistoryCapacity {
		// Drop oldest
		copy(h.groups, h.groups[1:])
		h.groups[len(h.groups)-1] = g
	} else {
		h.groups = append(h.groups, g)
	}
	h.cursor = len(h.groups) - 1
}

func (h *resultHistory) current() *resultGroup {
	if len(h.groups) == 0 || h.cursor < 0 || h.cursor >= len(h.groups) {
		return nil
	}
	return &h.groups[h.cursor]
}

func (h *resultHistory) prev() {
	if h.cursor > 0 {
		h.cursor--
	}
}

func (h *resultHistory) next() {
	if h.cursor < len(h.groups)-1 {
		h.cursor++
	}
}

func (h *resultHistory) len() int { return len(h.groups) }

func (h *resultHistory) isEmpty() bool { return len(h.groups) == 0 }

// index1 returns the 1-based index of the current group for display.
func (h *resultHistory) index1() int { return h.cursor + 1 }
