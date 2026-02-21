package main

import (
	"image"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

var (
	borderGrey   = lipgloss.NewStyle().Foreground(palette.Subtle)
	borderYellow = lipgloss.NewStyle().Foreground(palette.Yellow)
)

func (m model) View() tea.View {
	if m.width == 0 || m.height == 0 {
		return tea.View{
			Content:   "Loading...",
			AltScreen: true,
			MouseMode: tea.MouseModeAllMotion,
		}
	}

	canvas := uv.NewScreenBuffer(m.width, m.height)
	l := m.cachedLayout

	// Filter help overlay
	if m.overlay.kind == overlayFilterHelp {
		content := renderFilterHelp()
		m.overlay.drawCentered(canvas, l.area, content)
		return tea.View{
			Content:   canvas.Render(),
			AltScreen: true,
			MouseMode: tea.MouseModeAllMotion,
		}
	}

	// Full-screen help overlay
	if m.overlay.kind == overlayHelp {
		content := renderFullHelp()
		m.overlay.drawCentered(canvas, l.area, content)
		return tea.View{
			Content:   canvas.Render(),
			AltScreen: true,
			MouseMode: tea.MouseModeAllMotion,
		}
	}

	// Main pane content (shared with snapshot)
	m.renderPanes(canvas, l)

	// Chord hint popup
	if m.pendingChord != "" {
		for _, group := range chordGroups() {
			if group.prefix == m.pendingChord {
				content := renderChordHint(group)
				box := styles.Dialog.Box.Render(content)
				w := lipgloss.Width(box)
				h := lipgloss.Height(box)
				// Position bottom-center, above the hint bar
				x := (l.area.Dx() - w) / 2
				if x < 0 {
					x = 0
				}
				y := l.bottom.Min.Y - h
				if y < 0 {
					y = 0
				}
				rect := image.Rect(x, y, x+w, y+h)
				uv.NewStyledString(box).Draw(canvas, rect)
				break
			}
		}
	}

	// Connection dialog overlay
	if m.overlay.kind == overlayConnect && m.dialog != nil {
		m.overlay.drawCentered(canvas, l.area, m.dialog.view())
	}

	// Context menu
	if m.contextMenu.visible {
		m.contextMenu.draw(canvas, l.area)
	}

	// Tooltip (suppress when context menu is open)
	if m.tooltip.visible && !m.contextMenu.visible {
		m.tooltip.draw(canvas, l.area)
	}

	return tea.View{
		Content:   canvas.Render(),
		AltScreen: true,
		MouseMode: tea.MouseModeAllMotion,
	}
}

// renderPanes draws the header, borders, tree, top/bottom right panes, and
// bottom bar onto the canvas. This is the shared rendering core used by both
// View() and snapshot().
func (m model) renderPanes(canvas uv.ScreenBuffer, l appLayout) {
	// Header bar (crush-style)
	headerStr := m.renderHeader(l.header.Dx())
	uv.NewStyledString(headerStr).Draw(canvas, l.header)

	// Border frame around panes
	m.drawBorders(canvas, l)

	// Tree pane - unfocused when another major pane has focus
	treeFocused := m.focus != focusResults && m.focus != focusResultFilter && m.focus != focusDetail
	treeContent := styles.Tree.Pane.
		Width(l.tree.Dx()).
		Height(l.tree.Dy()).
		Render(m.tree.view(treeFocused))
	uv.NewStyledString(treeContent).Draw(canvas, l.tree)

	// Top-right sub-pane (detail/diag/table schema/module/xref picker)
	var topContent string
	if m.focus == focusXref {
		topContent = renderPane(l.rightTop, m.xrefPicker.view())
	} else {
		switch m.topPane {
		case topDiag:
			topContent = renderPane(l.rightTop, m.diag.view())
		case topModule:
			topContent = renderPane(l.rightTop, m.module.view())
		case topTypes:
			topContent = renderPane(l.rightTop, m.typeBrowser.view())
		case topTableSchema:
			topContent = renderPane(l.rightTop, m.tableSchema.view())
		default:
			m.detail.resultsFocused = m.focus == focusResults || m.focus == focusResultFilter
			topContent = renderPane(l.rightTop, m.detail.view(m.focus == focusDetail))
		}
	}
	uv.NewStyledString(topContent).Draw(canvas, l.rightTop)

	// Bottom-right sub-pane (results/table data)
	if l.rightBot.Dy() > 0 {
		var botContent string
		switch m.bottomPane {
		case bottomResults:
			botContent = renderPane(l.rightBot, m.results.view(m.focus == focusResults || m.focus == focusResultFilter))
		case bottomTableData:
			botContent = renderPane(l.rightBot, m.tableData.view())
		}
		if botContent != "" {
			uv.NewStyledString(botContent).Draw(canvas, l.rightBot)
		}
	}

	// Bottom bar (help/search/filter/query/status)
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
		// Persistent result filter indicator when in results focus
		bottom = m.renderResultFilterIndicator()
	} else if m.filterBar.isFiltering() {
		// Persistent filter indicator when filter is active but not focused
		bottom = m.renderFilterIndicator()
	} else if m.status.current != nil {
		bottom = m.status.view()
	} else {
		bottom = m.renderHintBar()
	}
	uv.NewStyledString(bottom).Draw(canvas, l.bottom)
}

func (m model) renderHeader(width int) string {
	// Brand
	brand := styles.Header.Brand.Render("gomib")

	// Diagonal fill
	diag := styles.Header.Diagonal.Render(" " + strings.Repeat(DiagFill, 3) + " ")

	// Breadcrumb of selected node
	var crumb string
	if node := m.tree.selectedNode(); node != nil {
		crumb = buildBreadcrumb(node)
		if crumb != "" {
			crumb += " "
		}
	}

	// Device pills (inline in header)
	var pills string
	if m.snmp.IsConnected() {
		pills = styles.Status.SuccessIcon.Render(IconPending) + " " +
			styles.Pill.Connected.Render(m.snmp.Target) + " " +
			styles.Pill.Version.Render("("+m.snmp.Version+")")
	}

	// Assemble
	leftPart := " " + brand + diag + crumb
	leftW := lipgloss.Width(leftPart)
	pillsW := lipgloss.Width(pills)

	gap := width - leftW - pillsW - 2
	if gap < 1 {
		gap = 1
	}

	var line string
	if pills != "" {
		line = leftPart + strings.Repeat(" ", gap) + pills + " "
	} else {
		// Show stats on the right when no device connected
		statsText := styles.Label.Render(m.stats)
		statsW := lipgloss.Width(statsText)
		gap = width - leftW - statsW - 2
		if gap < 1 {
			gap = 1
		}
		line = leftPart + strings.Repeat(" ", gap) + statsText + " "
	}

	return styles.Header.Bar.Width(width).Render(line)
}

// renderHintBar builds the two-line grouped keybind hints for the bottom bar.
// Line 1: Browse (navigation, search, filter, copy, tab)
// Line 2: Chord prefixes + meta
func (m model) renderHintBar() string {
	ks := styles.Value.Bold(true)
	ds := styles.Label
	sep := styles.Help.Sep.Render(" \u2502 ")

	h := func(key, desc string) string {
		return ks.Render(key) + " " + ds.Render(desc)
	}

	// Line 1: Browse
	line1 := h("\u2191\u2193", "nav") + "  " +
		h("/", "search") + "  " +
		h("f", "filter") + "  " +
		h("y", "copy") + "  " +
		h("tab", "focus")

	// Right-align stats on line 1
	statsText := styles.Label.Render(m.stats)
	line1W := lipgloss.Width(line1)
	statsW := lipgloss.Width(statsText)
	if gap := m.width - 2 - line1W - statsW; gap > 0 {
		line1 += strings.Repeat(" ", gap) + statsText
	}

	// Line 2: Chords + meta
	line2 := h("s", "snmp...") + "  " +
		h("c", "connect...") + "  " +
		h("v", "view...") +
		sep +
		h("?", "help") + "  " +
		h("q", "quit")

	return line1 + "\n" + line2
}

// renderActiveIndicator shows an active filter/search prompt, value, and match
// count on line 1, with the first line of the hint bar on line 2.
func (m model) renderActiveIndicator(prompt, value string, matchCount int) string {
	line1 := styles.Prompt.Render(prompt) +
		styles.Value.Render(value) +
		styles.Status.SuccessMsg.Render("  "+matchBadge(matchCount))

	line2 := m.renderHintBar()
	// Take only the first line of hint bar (browse line)
	if idx := strings.Index(line2, "\n"); idx >= 0 {
		line2 = line2[:idx]
	}
	return line1 + "\n" + line2
}

// renderFilterIndicator shows the active filter expression and match count
// when the filter bar is not focused.
func (m model) renderFilterIndicator() string {
	f := m.filterBar.filter
	return m.renderActiveIndicator("f ", f.expr, f.matchCount)
}

// renderResultFilterIndicator shows the active result filter and match count
// when the result filter input is not focused.
func (m model) renderResultFilterIndicator() string {
	return m.renderActiveIndicator("/ results: ", m.results.filterQuery, len(m.results.filterIdx))
}

// renderPane wraps content in the standard pane style sized to the given rect.
func renderPane(rect image.Rectangle, content string) string {
	return styles.Pane.
		Width(rect.Dx()).
		Height(rect.Dy()).
		Render(content)
}

// drawBorders renders thin box-drawing borders around each pane section.
// The border is grey by default and yellow for the focused section.
func (m model) drawBorders(canvas uv.ScreenBuffer, l appLayout) {
	active := m.activePaneID()

	// Border coordinates derived from content pane positions
	topY := l.tree.Min.Y - 1   // row above content
	botY := l.tree.Max.Y       // row below content
	leftX := l.tree.Min.X - 1  // column left of tree (= 0)
	rightX := l.rightTop.Max.X // column right of right pane
	midX := l.sep.Min.X        // shared vertical border column

	// Guard against degenerate layouts
	if botY <= topY || midX <= leftX+1 || rightX <= midX+1 {
		return
	}

	hasBot := l.rightBot.Dy() > 0
	hSepY := -1
	if hasBot {
		hSepY = l.rightSep.Min.Y
	}

	styleFor := func(panes ...paneID) lipgloss.Style {
		for _, p := range panes {
			if p == active {
				return borderYellow
			}
		}
		return borderGrey
	}

	ch := func(s string, panes ...paneID) string {
		return styleFor(panes...).Render(s)
	}

	// Which pane owns the bottom-right area
	botPane := paneRightTop
	if hasBot {
		botPane = paneRightBot
	}

	// Top border: ┌───┬───┐
	topLine := ch("\u250c", paneTree) +
		styleFor(paneTree).Render(strings.Repeat("\u2500", midX-leftX-1)) +
		ch("\u252c", paneTree, paneRightTop) +
		styleFor(paneRightTop).Render(strings.Repeat("\u2500", rightX-midX-1)) +
		ch("\u2510", paneRightTop)
	uv.NewStyledString(topLine).Draw(canvas, image.Rect(leftX, topY, rightX+1, topY+1))

	// Bottom border: └───┴───┘
	botLine := ch("\u2514", paneTree) +
		styleFor(paneTree).Render(strings.Repeat("\u2500", midX-leftX-1)) +
		ch("\u2534", paneTree, botPane) +
		styleFor(botPane).Render(strings.Repeat("\u2500", rightX-midX-1)) +
		ch("\u2518", botPane)
	uv.NewStyledString(botLine).Draw(canvas, image.Rect(leftX, botY, rightX+1, botY+1))

	// Left vertical border (tree)
	drawVerticalBorder(canvas, leftX, topY+1, botY, styleFor(paneTree))

	// Middle vertical border (shared between tree and right panes)
	if hasBot && hSepY > topY+1 {
		// Above horizontal separator
		drawVerticalBorder(canvas, midX, topY+1, hSepY, styleFor(paneTree, paneRightTop))
		// Below horizontal separator
		drawVerticalBorder(canvas, midX, hSepY+1, botY, styleFor(paneTree, paneRightBot))
	} else {
		drawVerticalBorder(canvas, midX, topY+1, botY, styleFor(paneTree, paneRightTop))
	}

	// Right vertical border
	if hasBot && hSepY > topY+1 {
		drawVerticalBorder(canvas, rightX, topY+1, hSepY, styleFor(paneRightTop))
		drawVerticalBorder(canvas, rightX, hSepY+1, botY, styleFor(paneRightBot))
	} else {
		drawVerticalBorder(canvas, rightX, topY+1, botY, styleFor(paneRightTop))
	}

	// Horizontal separator between right-top and right-bot: ├───┤
	if hasBot {
		hLine := ch("\u251c", paneTree, paneRightTop, paneRightBot) +
			styleFor(paneRightTop, paneRightBot).Render(strings.Repeat("\u2500", rightX-midX-1)) +
			ch("\u2524", paneRightTop, paneRightBot)
		uv.NewStyledString(hLine).Draw(canvas, image.Rect(midX, hSepY, rightX+1, hSepY+1))
	}
}

// drawVerticalBorder draws │ characters in a column from startY (inclusive) to endY (exclusive).
func drawVerticalBorder(canvas uv.ScreenBuffer, x, startY, endY int, style lipgloss.Style) {
	h := endY - startY
	if h <= 0 {
		return
	}
	var b strings.Builder
	ch := style.Render("\u2502")
	for i := range h {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(ch)
	}
	uv.NewStyledString(b.String()).Draw(canvas, image.Rect(x, startY, x+1, endY))
}

// renderFullHelp builds a custom help overlay showing globals, chords, and
// results-only keys.
func renderFullHelp() string {
	hdr := styles.Header.Info
	ks := styles.Value.Bold(true)
	ds := styles.Label

	h := func(key, desc string) string {
		return "  " + ks.Render(key) + "  " + ds.Render(desc)
	}

	var b strings.Builder

	b.WriteString(hdr.Render("Navigation"))
	b.WriteString("\n")
	b.WriteString(h("j/k, \u2191\u2193", "up/down"))
	b.WriteString("\n")
	b.WriteString(h("ctrl+d/u, pgdn/up", "page down/up"))
	b.WriteString("\n")
	b.WriteString(h("home, G/end", "top/bottom"))
	b.WriteString("\n")
	b.WriteString(h("enter/l/\u2192", "expand"))
	b.WriteString("\n")
	b.WriteString(h("h/\u2190", "collapse"))
	b.WriteString("\n")
	b.WriteString(h("J/K", "scroll detail pane"))
	b.WriteString("\n")
	b.WriteString(h("tab", "cycle pane focus"))
	b.WriteString("\n")
	b.WriteString(h("bksp", "back (after jump)"))
	b.WriteString("\n\n")

	b.WriteString(hdr.Render("Global"))
	b.WriteString("\n")
	b.WriteString(h("/", "search (tree) / filter (results)"))
	b.WriteString("\n")
	b.WriteString(h("f", "CEL filter"))
	b.WriteString("\n")
	b.WriteString(h("y", "copy OID"))
	b.WriteString("\n")
	b.WriteString(h("x", "cross-refs"))
	b.WriteString("\n")
	b.WriteString(h("[/]", "results history"))
	b.WriteString("\n")
	b.WriteString(h("</> ", "table data scroll"))
	b.WriteString("\n\n")

	// Chord groups
	for _, group := range chordGroups() {
		b.WriteString(hdr.Render(group.label + " (" + group.prefix + " ...)"))
		b.WriteString("\n")
		for _, a := range group.actions {
			b.WriteString(h(group.prefix+" "+a.key, a.label))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString(hdr.Render("Results only"))
	b.WriteString("\n")
	b.WriteString(h("enter", "cross-reference / jump"))
	b.WriteString("\n")
	b.WriteString(h("h/l", "collapse/expand tree"))
	b.WriteString("\n\n")

	b.WriteString(h("?", "this help"))
	b.WriteString("\n")
	b.WriteString(h("q, ctrl+c", "quit"))

	return b.String()
}
