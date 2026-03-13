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
	fmt.Fprintf(&b, "Time: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(&b, "Terminal: %dx%d\n", m.width, m.height)
	fmt.Fprintf(&b, "Focus: %s\n", m.focus)
	fmt.Fprintf(&b, "Top pane: %s\n", m.topPane)
	fmt.Fprintf(&b, "Bottom pane: %s\n", m.bottomPane)

	if node := m.tree.selectedNode(); node != nil {
		fmt.Fprintf(&b, "Selected node: %s (%s)\n", node.Name(), node.OID())
	}
	if m.snmp.IsConnected() {
		fmt.Fprintf(&b, "Connected: %s (%s)\n", m.snmp.Target, m.snmp.Version)
	} else {
		b.WriteString("Connected: no\n")
	}
	if m.pendingChord != "" {
		fmt.Fprintf(&b, "Pending chord: %q\n", m.pendingChord)
	}
	if m.results.isFiltering() {
		fmt.Fprintf(&b, "Result filter: %q (%d matches)\n", m.results.filterQuery, len(m.results.filterIdx))
	}
	if m.filterBar.isFiltering() {
		fmt.Fprintf(&b, "Tree filter: %q\n", m.filterBar.input.Value())
	}

	b.WriteString("\nLayout:\n")
	fmt.Fprintf(&b, "  tree:     (%d,%d)-(%d,%d) %dx%d\n", l.tree.Min.X, l.tree.Min.Y, l.tree.Max.X, l.tree.Max.Y, l.tree.Dx(), l.tree.Dy())
	fmt.Fprintf(&b, "  sep:      (%d,%d)-(%d,%d)\n", l.sep.Min.X, l.sep.Min.Y, l.sep.Max.X, l.sep.Max.Y)
	fmt.Fprintf(&b, "  rightTop: (%d,%d)-(%d,%d) %dx%d\n", l.rightTop.Min.X, l.rightTop.Min.Y, l.rightTop.Max.X, l.rightTop.Max.Y, l.rightTop.Dx(), l.rightTop.Dy())
	if l.rightSep.Dy() > 0 {
		fmt.Fprintf(&b, "  rightSep: (%d,%d)-(%d,%d)\n", l.rightSep.Min.X, l.rightSep.Min.Y, l.rightSep.Max.X, l.rightSep.Max.Y)
		fmt.Fprintf(&b, "  rightBot: (%d,%d)-(%d,%d) %dx%d\n", l.rightBot.Min.X, l.rightBot.Min.Y, l.rightBot.Max.X, l.rightBot.Max.Y, l.rightBot.Dx(), l.rightBot.Dy())
	}
	fmt.Fprintf(&b, "  bottom:   (%d,%d)-(%d,%d)\n", l.bottom.Min.X, l.bottom.Min.Y, l.bottom.Max.X, l.bottom.Max.Y)

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
