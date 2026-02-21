package main

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"github.com/golangsnmp/gomib/mib"
)

const maxSearchVisible = 10

// searchEntry is a pre-built index entry for search.
type searchEntry struct {
	node    *mib.Node
	nameLow string // lowercase name for case-insensitive match
	oidStr  string
	descLow string // lowercase description for content search
	descHit bool   // true when match came from description, not name
}

// searchModel is the search overlay component.
type searchModel struct {
	input  textinput.Model
	index  []searchEntry
	list   ListView[searchEntry]
	active bool
}

func newSearchModel(m *mib.Mib) searchModel {
	ti := newStyledInput("/ ", 128)

	// Build search index
	var index []searchEntry
	for node := range m.Nodes() {
		name := node.Name()
		oid := node.OID().String()
		_, desc, _ := nodeEntityProps(node)
		index = append(index, searchEntry{
			node:    node,
			nameLow: strings.ToLower(name),
			oidStr:  oid,
			descLow: strings.ToLower(desc),
		})
	}

	sm := searchModel{
		input: ti,
		index: index,
	}
	sm.list.SetSize(0, maxSearchVisible)
	return sm
}

func (s *searchModel) setSize(width int) {
	s.list.SetSize(width, maxSearchVisible)
}

func (s *searchModel) activate() {
	s.active = true
	s.input.SetValue("")
	s.input.Focus()
	s.list.SetRows(nil)
}

func (s *searchModel) deactivate() {
	s.active = false
	s.input.Blur()
	s.list.SetRows(nil)
}

func (s *searchModel) filter() {
	query := s.input.Value()
	if query == "" {
		s.list.SetRows(nil)
		return
	}

	var results []searchEntry
	queryLow := strings.ToLower(query)

	// OID prefix match if query starts with digit or dot
	isOID := len(query) > 0 && (query[0] >= '0' && query[0] <= '9' || query[0] == '.')

	// Collect name/OID matches first, then description-only matches
	var descMatches []searchEntry
	for i := range s.index {
		entry := &s.index[i]
		if isOID {
			if strings.HasPrefix(entry.oidStr, queryLow) {
				results = append(results, *entry)
			}
		} else {
			if strings.Contains(entry.nameLow, queryLow) {
				e := *entry
				e.descHit = false
				results = append(results, e)
			} else if entry.descLow != "" && strings.Contains(entry.descLow, queryLow) {
				e := *entry
				e.descHit = true
				descMatches = append(descMatches, e)
			}
		}
	}
	// Append description matches after name matches
	results = append(results, descMatches...)

	s.list.SetRows(results)
}

func (s *searchModel) selectedNode() *mib.Node {
	if entry := s.list.Selected(); entry != nil {
		return entry.node
	}
	return nil
}

func (s *searchModel) nextResult() {
	s.list.CursorDown()
}

func (s *searchModel) prevResult() {
	s.list.CursorUp()
}

func (s *searchModel) view() string {
	if !s.active {
		return ""
	}

	var b strings.Builder
	b.WriteString(s.input.View())

	// Count badge when there are more results than visible
	if s.list.Len() > s.list.VisibleRows() {
		badge := fmt.Sprintf(" %d/%d ", s.list.Cursor()+1, s.list.Len())
		b.WriteString("  " + styles.StatusText.Render(badge))
	}
	b.WriteByte('\n')

	b.WriteString(s.list.Render(renderSearchRow))

	return b.String()
}

// renderSearchRow is the RenderFunc for search result rows.
func renderSearchRow(entry searchEntry, _ int, selected bool, width int) string {
	name := entry.node.Name()
	if name == "" {
		name = "(" + entry.oidStr + ")"
	}
	kindDot := kindStyle(entry.node.Kind()).Render(IconPending)
	descTag := ""
	if entry.descHit {
		descTag = " " + styles.Subtle.Render("(desc)")
	}

	if selected {
		line := kindDot + " " + name + "  " + entry.oidStr + descTag
		return renderSelectedRow(line, width)
	}
	return "  " + kindDot + " " + name + "  " + styles.Label.Render(entry.oidStr) + descTag
}
