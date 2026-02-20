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
	input    textinput.Model
	index    []searchEntry
	results  []searchEntry
	selected int
	offset   int // first visible result index
	active   bool
	width    int
}

func newSearchModel(m *mib.Mib) searchModel {
	ti := newStyledInput("/ ", 128)

	// Build search index
	var index []searchEntry
	for node := range m.Nodes() {
		name := node.Name()
		oid := node.OID().String()
		index = append(index, searchEntry{
			node:    node,
			nameLow: strings.ToLower(name),
			oidStr:  oid,
			descLow: strings.ToLower(nodeDescription(node)),
		})
	}

	return searchModel{
		input: ti,
		index: index,
	}
}

func (s *searchModel) setSize(width int) {
	s.width = width
}

func (s *searchModel) activate() {
	s.active = true
	s.input.SetValue("")
	s.input.Focus()
	s.results = nil
	s.selected = 0
	s.offset = 0
}

func (s *searchModel) deactivate() {
	s.active = false
	s.input.Blur()
	s.results = nil
}

func (s *searchModel) filter() {
	query := s.input.Value()
	if query == "" {
		s.results = nil
		s.selected = 0
		s.offset = 0
		return
	}

	s.results = s.results[:0]
	queryLow := strings.ToLower(query)

	// OID prefix match if query starts with digit or dot
	isOID := len(query) > 0 && (query[0] >= '0' && query[0] <= '9' || query[0] == '.')

	// Collect name/OID matches first, then description-only matches
	var descMatches []searchEntry
	for i := range s.index {
		entry := &s.index[i]
		if isOID {
			if strings.HasPrefix(entry.oidStr, queryLow) {
				s.results = append(s.results, *entry)
			}
		} else {
			if strings.Contains(entry.nameLow, queryLow) {
				e := *entry
				e.descHit = false
				s.results = append(s.results, e)
			} else if entry.descLow != "" && strings.Contains(entry.descLow, queryLow) {
				e := *entry
				e.descHit = true
				descMatches = append(descMatches, e)
			}
		}
	}
	// Append description matches after name matches
	s.results = append(s.results, descMatches...)

	if s.selected >= len(s.results) {
		s.selected = max(0, len(s.results)-1)
	}
	s.ensureVisible()
}

func (s *searchModel) selectedNode() *mib.Node {
	if s.selected >= 0 && s.selected < len(s.results) {
		return s.results[s.selected].node
	}
	return nil
}

func (s *searchModel) nextResult() {
	if s.selected < len(s.results)-1 {
		s.selected++
		s.ensureVisible()
	}
}

func (s *searchModel) prevResult() {
	if s.selected > 0 {
		s.selected--
		s.ensureVisible()
	}
}

func (s *searchModel) ensureVisible() {
	if s.selected < s.offset {
		s.offset = s.selected
	}
	if s.selected >= s.offset+maxSearchVisible {
		s.offset = s.selected - maxSearchVisible + 1
	}
}

func (s *searchModel) view() string {
	if !s.active {
		return ""
	}

	var b strings.Builder
	b.WriteString(s.input.View())

	// Count badge when there are more results than visible
	if len(s.results) > maxSearchVisible {
		badge := fmt.Sprintf(" %d/%d ", s.selected+1, len(s.results))
		b.WriteString("  " + styles.StatusText.Render(badge))
	}
	b.WriteByte('\n')

	end := min(s.offset+maxSearchVisible, len(s.results))
	for i := s.offset; i < end; i++ {
		entry := s.results[i]
		name := entry.node.Name()
		if name == "" {
			name = "(" + entry.oidStr + ")"
		}
		kindDot := kindStyle(entry.node.Kind()).Render(IconPending)
		descTag := ""
		if entry.descHit {
			descTag = " " + styles.Subtle.Render("(desc)")
		}

		if i == s.selected {
			line := kindDot + " " + name + "  " + entry.oidStr + descTag
			b.WriteString(renderSelectedRow(line, s.width))
		} else {
			line := "  " + kindDot + " " + name + "  " + styles.Label.Render(entry.oidStr) + descTag
			b.WriteString(line)
		}
		if i < end-1 {
			b.WriteByte('\n')
		}
	}

	return b.String()
}
