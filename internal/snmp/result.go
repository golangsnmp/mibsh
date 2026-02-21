package snmp

import (
	"github.com/golangsnmp/gomib/mib"
)

// OpKind identifies the type of SNMP operation that produced a result group.
type OpKind int

const (
	OpGet OpKind = iota
	OpGetNext
	OpWalk
)

// Result is a single formatted SNMP result.
type Result struct {
	OID      string // dotted OID string
	Name     string // resolved name (e.g. "sysDescr.0")
	Value    string // formatted value
	TypeName string // type label (e.g. "STRING", "INTEGER")
}

// ResultGroup is a set of results from a single SNMP operation.
type ResultGroup struct {
	Op          OpKind
	Label       string // short description (e.g. "GET sysDescr.0")
	Results     []Result
	Err         error   // non-nil if the operation failed
	WalkRootOID mib.OID // root OID for walk operations (used by tree view)
}

const resultHistoryCapacity = 50

// ResultHistory is a capped slice of result groups.
type ResultHistory struct {
	groups []ResultGroup
	cursor int // index of the currently viewed group (-1 if empty)
}

func (h *ResultHistory) Add(g ResultGroup) {
	if len(h.groups) >= resultHistoryCapacity {
		// Drop oldest
		copy(h.groups, h.groups[1:])
		h.groups[len(h.groups)-1] = g
	} else {
		h.groups = append(h.groups, g)
	}
	h.cursor = len(h.groups) - 1
}

func (h *ResultHistory) Current() *ResultGroup {
	if len(h.groups) == 0 || h.cursor < 0 || h.cursor >= len(h.groups) {
		return nil
	}
	return &h.groups[h.cursor]
}

func (h *ResultHistory) Prev() {
	if h.cursor > 0 {
		h.cursor--
	}
}

func (h *ResultHistory) Next() {
	if h.cursor < len(h.groups)-1 {
		h.cursor++
	}
}

func (h *ResultHistory) Len() int { return len(h.groups) }

func (h *ResultHistory) IsEmpty() bool { return len(h.groups) == 0 }

// Index1 returns the 1-based index of the current group for display.
func (h *ResultHistory) Index1() int { return h.cursor + 1 }
