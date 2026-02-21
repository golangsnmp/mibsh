package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// snapshotMsg signals that a snapshot was written.
type snapshotMsg struct {
	path string
	err  error
}

func (m model) snapshot() (tea.Model, tea.Cmd) {
	// Re-render to a fresh canvas so we get the exact current state
	canvas := uv.NewScreenBuffer(m.width, m.height)
	l := m.cachedLayout

	// Draw the same content as View()
	m.renderPanes(canvas, l)

	// Build the snapshot text
	var b strings.Builder

	b.WriteString("=== gomib TUI snapshot ===\n")
	b.WriteString(fmt.Sprintf("Time: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	b.WriteString(fmt.Sprintf("Terminal: %dx%d\n", m.width, m.height))
	b.WriteString(fmt.Sprintf("Focus: %s\n", m.focus))
	b.WriteString(fmt.Sprintf("Top pane: %s\n", m.topPane))
	b.WriteString(fmt.Sprintf("Bottom pane: %s\n", m.bottomPane))

	if node := m.tree.selectedNode(); node != nil {
		b.WriteString(fmt.Sprintf("Selected node: %s (%s)\n", node.Name(), node.OID()))
	}
	if m.snmp.IsConnected() {
		b.WriteString(fmt.Sprintf("Connected: %s (%s)\n", m.snmp.Target, m.snmp.Version))
	} else {
		b.WriteString("Connected: no\n")
	}
	if m.pendingChord != "" {
		b.WriteString(fmt.Sprintf("Pending chord: %q\n", m.pendingChord))
	}
	if m.results.isFiltering() {
		b.WriteString(fmt.Sprintf("Result filter: %q (%d matches)\n", m.results.filterQuery, len(m.results.filterIdx)))
	}
	if m.filterBar.isFiltering() {
		b.WriteString(fmt.Sprintf("Tree filter: %q\n", m.filterBar.input.Value()))
	}

	b.WriteString("\nLayout:\n")
	b.WriteString(fmt.Sprintf("  tree:     (%d,%d)-(%d,%d) %dx%d\n", l.tree.Min.X, l.tree.Min.Y, l.tree.Max.X, l.tree.Max.Y, l.tree.Dx(), l.tree.Dy()))
	b.WriteString(fmt.Sprintf("  sep:      (%d,%d)-(%d,%d)\n", l.sep.Min.X, l.sep.Min.Y, l.sep.Max.X, l.sep.Max.Y))
	b.WriteString(fmt.Sprintf("  rightTop: (%d,%d)-(%d,%d) %dx%d\n", l.rightTop.Min.X, l.rightTop.Min.Y, l.rightTop.Max.X, l.rightTop.Max.Y, l.rightTop.Dx(), l.rightTop.Dy()))
	if l.rightSep.Dy() > 0 {
		b.WriteString(fmt.Sprintf("  rightSep: (%d,%d)-(%d,%d)\n", l.rightSep.Min.X, l.rightSep.Min.Y, l.rightSep.Max.X, l.rightSep.Max.Y))
		b.WriteString(fmt.Sprintf("  rightBot: (%d,%d)-(%d,%d) %dx%d\n", l.rightBot.Min.X, l.rightBot.Min.Y, l.rightBot.Max.X, l.rightBot.Max.Y, l.rightBot.Dx(), l.rightBot.Dy()))
	}
	b.WriteString(fmt.Sprintf("  bottom:   (%d,%d)-(%d,%d)\n", l.bottom.Min.X, l.bottom.Min.Y, l.bottom.Max.X, l.bottom.Max.Y))

	b.WriteString("\n=== Screen (plain text) ===\n")
	// Use Buffer.String() for plain text (no ANSI)
	plainText := canvas.String()
	b.WriteString(plainText)
	b.WriteByte('\n')

	return m, func() tea.Msg {
		f, err := os.CreateTemp("", "mibsh-snapshot-*.txt")
		if err != nil {
			return snapshotMsg{err: err}
		}
		path := f.Name()
		_, err = f.WriteString(b.String())
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
		return snapshotMsg{path: path, err: err}
	}
}

func (f focus) String() string {
	switch f {
	case focusTree:
		return "tree"
	case focusSearch:
		return "search"
	case focusFilter:
		return "filter"
	case focusDiag:
		return "diagnostics"
	case focusModule:
		return "module"
	case focusQueryBar:
		return "query-bar"
	case focusResults:
		return "results"
	case focusResultFilter:
		return "result-filter"
	case focusDetail:
		return "detail"
	case focusTypes:
		return "types"
	case focusXref:
		return "xref"
	default:
		return fmt.Sprintf("unknown(%d)", f)
	}
}

func (p topPane) String() string {
	switch p {
	case topDetail:
		return "detail"
	case topDiag:
		return "diagnostics"
	case topTableSchema:
		return "table-schema"
	case topModule:
		return "module"
	case topTypes:
		return "types"
	default:
		return fmt.Sprintf("unknown(%d)", p)
	}
}

func (p bottomPane) String() string {
	switch p {
	case bottomNone:
		return "none"
	case bottomResults:
		return "results"
	case bottomTableData:
		return "table-data"
	default:
		return fmt.Sprintf("unknown(%d)", p)
	}
}
