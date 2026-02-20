package main

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"github.com/golangsnmp/gomib/mib"
)

// newStyledInput creates a textinput with the standard prompt styling applied.
func newStyledInput(prompt string, charLimit int) textinput.Model {
	ti := textinput.New()
	ti.Prompt = prompt
	ti.CharLimit = charLimit
	s := ti.Styles()
	s.Focused.Prompt = styles.Prompt
	s.Blurred.Prompt = styles.Prompt
	ti.SetStyles(s)
	return ti
}

// tabCompleter handles prefix-based tab completion with match cycling.
type tabCompleter struct {
	matches    []string
	matchIdx   int
	lastPrefix string
}

func (tc *tabCompleter) reset() {
	tc.matches = nil
	tc.matchIdx = 0
	tc.lastPrefix = ""
}

// complete returns the next match for the given prefix from candidates.
// When the prefix changes, matches are rebuilt. Returns empty string and
// false if no candidates match.
func (tc *tabCompleter) complete(prefix string, candidates []string) (string, bool) {
	if prefix != tc.lastPrefix {
		tc.lastPrefix = prefix
		tc.matches = tc.matches[:0]
		tc.matchIdx = 0
		prefixLow := strings.ToLower(prefix)
		for _, c := range candidates {
			if strings.HasPrefix(strings.ToLower(c), prefixLow) {
				tc.matches = append(tc.matches, c)
			}
		}
	}
	if len(tc.matches) == 0 {
		return "", false
	}
	result := tc.matches[tc.matchIdx]
	tc.matchIdx = (tc.matchIdx + 1) % len(tc.matches)
	return result, true
}

// indexNameSet builds a set of index column names from table indexes.
func indexNameSet(indexes []mib.IndexEntry) map[string]bool {
	set := make(map[string]bool, len(indexes))
	for _, idx := range indexes {
		if idx.Object != nil {
			set[idx.Object.Name()] = true
		}
	}
	return set
}

// nodeDescription returns the description text for a node, checking
// Object, Notification, Group, Compliance, and Capability in order.
func nodeDescription(node *mib.Node) string {
	if obj := node.Object(); obj != nil {
		return obj.Description()
	}
	if notif := node.Notification(); notif != nil {
		return notif.Description()
	}
	if grp := node.Group(); grp != nil {
		return grp.Description()
	}
	if comp := node.Compliance(); comp != nil {
		return comp.Description()
	}
	if cap := node.Capability(); cap != nil {
		return cap.Description()
	}
	return ""
}

// selectedBorder returns the rendered thick left-border glyph used as a
// selection indicator in focused rows.
func selectedBorder() string {
	return styles.Tree.FocusBorder.Render(BorderThick)
}

// renderSelectedRow renders a row with the focused selection border indicator.
// It prepends the thick left-border glyph and pads the result to the given width.
func renderSelectedRow(text string, width int) string {
	return padRight(selectedBorder()+" "+text, width)
}

// matchBadge returns a formatted "N matches" or "1 match" string.
func matchBadge(n int) string {
	if n == 1 {
		return "1 match"
	}
	return fmt.Sprintf("%d matches", n)
}

// formatIndexList formats a list of index entries as "[IMPLIED name, name, ...]".
func formatIndexList(indexes []mib.IndexEntry) string {
	names := make([]string, 0, len(indexes))
	for _, idx := range indexes {
		name := "(unknown)"
		if idx.Object != nil {
			name = idx.Object.Name()
		}
		if idx.Implied {
			name = "IMPLIED " + name
		}
		names = append(names, name)
	}
	return "[" + strings.Join(names, ", ") + "]"
}

// resolveTable resolves a table object from a table, row, or column node.
// Returns the table object and the column name (empty for non-column nodes).
func resolveTable(obj *mib.Object, kind mib.Kind) (*mib.Object, string) {
	switch kind {
	case mib.KindTable:
		return obj, ""
	case mib.KindRow:
		return obj.Table(), ""
	case mib.KindColumn:
		return obj.Table(), obj.Name()
	default:
		return nil, ""
	}
}
