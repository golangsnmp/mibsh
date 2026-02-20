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
	l := m.generateLayout()

	// Draw the same content as View()
	headerStr := m.renderHeader(l.header.Dx())
	uv.NewStyledString(headerStr).Draw(canvas, l.header)

	// Border frame
	m.drawBorders(canvas, l)

	treeFocused := m.focus != focusResults && m.focus != focusResultFilter && m.focus != focusDetail
	treeContent := styles.Tree.Pane.
		Width(l.tree.Dx()).
		Height(l.tree.Dy()).
		Render(m.tree.view(treeFocused))
	uv.NewStyledString(treeContent).Draw(canvas, l.tree)

	var topContent string
	if m.focus == focusXref {
		topContent = styles.Pane.Width(l.rightTop.Dx()).Height(l.rightTop.Dy()).Render(m.xrefPicker.view())
	} else {
		switch m.topPane {
		case topDiag:
			topContent = styles.Pane.Width(l.rightTop.Dx()).Height(l.rightTop.Dy()).Render(m.diag.view())
		case topModule:
			topContent = styles.Pane.Width(l.rightTop.Dx()).Height(l.rightTop.Dy()).Render(m.module.view())
		case topTypes:
			topContent = styles.Pane.Width(l.rightTop.Dx()).Height(l.rightTop.Dy()).Render(m.typeBrowser.view())
		case topTableSchema:
			topContent = styles.Pane.Width(l.rightTop.Dx()).Height(l.rightTop.Dy()).Render(m.tableSchema.view())
		default:
			m.detail.resultsFocused = m.focus == focusResults || m.focus == focusResultFilter
			topContent = styles.Pane.Width(l.rightTop.Dx()).Height(l.rightTop.Dy()).Render(m.detail.view(m.focus == focusDetail))
		}
	}
	uv.NewStyledString(topContent).Draw(canvas, l.rightTop)

	if l.rightBot.Dy() > 0 {
		var botContent string
		switch m.bottomPane {
		case bottomResults:
			botContent = styles.Pane.Width(l.rightBot.Dx()).Height(l.rightBot.Dy()).
				Render(m.results.view(m.focus == focusResults || m.focus == focusResultFilter))
		case bottomTableData:
			botContent = styles.Pane.Width(l.rightBot.Dx()).Height(l.rightBot.Dy()).Render(m.tableData.view())
		}
		if botContent != "" {
			uv.NewStyledString(botContent).Draw(canvas, l.rightBot)
		}
	}

	var bottom string
	if m.focus == focusSearch {
		bottom = m.search.view()
	} else if m.focus == focusResultFilter {
		bottom = m.results.filterView()
	} else if m.focus == focusFilter {
		bottom = m.filterBar.view()
	} else if m.focus == focusQueryBar {
		bottom = m.queryBar.view()
	} else if m.results.isFiltering() && m.focus == focusResults {
		bottom = m.renderResultFilterIndicator()
	} else if m.filterBar.isFiltering() {
		bottom = m.renderFilterIndicator()
	} else if m.status.current != nil {
		bottom = m.status.view(m.width - 2)
	} else {
		bottom = m.renderHintBar()
	}
	uv.NewStyledString(bottom).Draw(canvas, l.bottom)

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
	if m.snmp != nil && m.snmp.connected {
		b.WriteString(fmt.Sprintf("Connected: %s (%s)\n", m.snmp.target, m.snmp.version))
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
