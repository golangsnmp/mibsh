package main

import (
	"errors"
	"sort"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/golangsnmp/gomib/mib"
)

// queryOp is the SNMP operation to perform.
type queryOp int

const (
	queryGet queryOp = iota
	queryGetNext
	queryWalk
)

// queryCmd represents a parsed query bar command.
type queryCmd struct {
	op  queryOp
	oid string // resolved dotted OID string
}

// queryBarModel is the bottom-bar command input for direct SNMP queries.
type queryBarModel struct {
	input textinput.Model
	mib   *mib.Mib
	err   string // validation/resolution error

	names []string     // sorted node names for completion
	tc    tabCompleter // tab completion state
}

func newQueryBar(m *mib.Mib) queryBarModel {
	ti := newStyledInput(": ", 256)
	ti.Placeholder = "get|walk|next NAME or OID (tab to complete)"
	s := ti.Styles()
	s.Cursor = textinput.CursorStyle{
		Color: palette.Primary,
		Shape: tea.CursorBar,
		Blink: true,
	}
	ti.SetStyles(s)

	// Build sorted name list for tab completion
	var names []string
	for node := range m.Nodes() {
		name := node.Name()
		if name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	return queryBarModel{
		input: ti,
		mib:   m,
		names: names,
	}
}

func (q *queryBarModel) activate() {
	q.input.SetValue("")
	q.err = ""
	q.resetCompletion()
}

func (q *queryBarModel) focusCmd() tea.Cmd {
	return q.input.Focus()
}

func (q *queryBarModel) blur() {
	q.input.Blur()
}

func (q *queryBarModel) resetCompletion() {
	q.tc.reset()
}

// parse parses the input text into a queryCmd.
// Returns nil and sets q.err on failure.
func (q *queryBarModel) parse() *queryCmd {
	text := strings.TrimSpace(q.input.Value())
	if text == "" {
		q.err = ""
		return nil
	}

	op := queryGet
	arg := text

	// Check for command prefix
	parts := strings.SplitN(text, " ", 2)
	if len(parts) == 2 {
		switch strings.ToLower(parts[0]) {
		case "get":
			op = queryGet
			arg = strings.TrimSpace(parts[1])
		case "next", "getnext":
			op = queryGetNext
			arg = strings.TrimSpace(parts[1])
		case "walk":
			op = queryWalk
			arg = strings.TrimSpace(parts[1])
		default:
			// Not a recognized command, treat entire text as the target
		}
	}

	if arg == "" {
		q.err = "missing OID or name"
		return nil
	}

	// Resolve the argument to a dotted OID string
	oidStr, err := q.resolve(arg)
	if err != nil {
		q.err = err.Error()
		return nil
	}

	q.err = ""
	return &queryCmd{op: op, oid: oidStr}
}

// resolve tries to resolve a name or OID string to a dotted OID.
func (q *queryBarModel) resolve(s string) (string, error) {
	// Try as dotted OID first (starts with digit or dot)
	if len(s) > 0 && (s[0] >= '0' && s[0] <= '9' || s[0] == '.') {
		oid, err := mib.ParseOID(s)
		if err != nil {
			return "", errors.New("invalid OID: " + s)
		}
		return oid.String(), nil
	}

	// Strip instance suffix (e.g. "sysDescr.0" -> name="sysDescr", suffix=".0")
	name := s
	suffix := ""
	if idx := strings.IndexByte(s, '.'); idx >= 0 {
		name = s[:idx]
		suffix = s[idx:]
	}

	// Try exact name lookup
	node := q.mib.Node(name)
	if node == nil {
		return "", errors.New("unknown: " + s)
	}

	oid := node.OID()
	if oid == nil {
		return "", errors.New("no OID for: " + name)
	}

	return oid.String() + suffix, nil
}

// tabComplete performs tab completion on the current input.
func (q *queryBarModel) tabComplete() {
	text := q.input.Value()

	// Extract the argument portion (after command prefix)
	prefix := text
	cmdPrefix := ""
	parts := strings.SplitN(text, " ", 2)
	if len(parts) == 2 {
		switch strings.ToLower(parts[0]) {
		case "get", "next", "getnext", "walk":
			cmdPrefix = parts[0] + " "
			prefix = parts[1]
		}
	}

	// Don't complete OIDs (starting with digit or dot)
	if len(prefix) > 0 && (prefix[0] >= '0' && prefix[0] <= '9' || prefix[0] == '.') {
		return
	}

	if prefix == "" {
		return
	}

	completion, ok := q.tc.complete(prefix, q.names)
	if !ok {
		return
	}

	q.input.SetValue(cmdPrefix + completion)
	q.input.CursorEnd()
}

func (q *queryBarModel) view() string {
	var b strings.Builder
	b.WriteString(q.input.View())
	if q.err != "" {
		b.WriteString("  ")
		b.WriteString(styles.Status.ErrorMsg.Render(q.err))
	}
	return b.String()
}
