package main

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
	"github.com/golangsnmp/gomib/mib"
)

// resultModel displays SNMP operation results in the bottom-right pane.
type resultModel struct {
	history    resultHistory
	flatLV     ListView[struct{}] // flat mode cursor/offset/scroll
	width      int
	height     int
	walkStatus string // "", "walking...", "cancelling..."
	focused    bool   // set during view() to control border style

	// Display modes
	showRawOID bool
	treeMode   bool
	resultTree *resultTreeNode
	treeLV     ListView[resultTreeRow]
	mib        *mib.Mib

	// Result filtering (activated by "/" while results are focused)
	filterInput textinput.Model
	filterQuery string // current filter substring (lowercased)
	filterIdx   []int  // indices into results that match the filter (flat mode)
}

func newResultModel() resultModel {
	return resultModel{
		flatLV:      NewListView[struct{}](2),
		treeLV:      NewListView[resultTreeRow](2),
		filterInput: newStyledInput("/ results: ", 128),
	}
}

func (r *resultModel) setSize(w, h int) {
	r.width = w
	r.height = h
	r.flatLV.SetSize(w, h)
	r.treeLV.SetSize(w, h)
}

func (r *resultModel) addGroup(g resultGroup) {
	r.history.add(g)
	r.resetView()
}

func (r *resultModel) resetView() {
	r.filterQuery = ""
	r.filterIdx = nil
	r.resultTree = nil
	r.flatLV.SetRows(nil)
	r.treeLV.SetRows(nil)
	if r.treeMode {
		r.rebuildTree()
	}
}

func (r *resultModel) appendResults(results []snmpResult) {
	g := r.history.current()
	if g == nil {
		return
	}
	g.results = append(g.results, results...)

	// Update tree if in tree mode
	if r.treeMode && r.resultTree != nil && r.mib != nil {
		for i := range results {
			insertResultIntoTree(r.resultTree, &g.results[len(g.results)-len(results)+i], g.walkRootOID, r.mib)
		}
		r.rebuildTreeRows()
	}
}

func (r *resultModel) totalRows() int {
	if r.treeMode {
		return r.treeLV.Len()
	}
	return r.filteredTotalRows()
}

// syncFlatRows updates the flat ListView row count to match the current
// filtered total, preserving cursor position.
func (r *resultModel) syncFlatRows() {
	total := r.filteredTotalRows()
	rows := make([]struct{}, total)
	r.flatLV.SetRows(rows)
}

func (r *resultModel) cursorDown() {
	if r.treeMode {
		r.treeLV.CursorDown()
		return
	}
	r.syncFlatRows()
	r.flatLV.CursorDown()
}

func (r *resultModel) cursorUp() {
	if r.treeMode {
		r.treeLV.CursorUp()
		return
	}
	r.syncFlatRows()
	r.flatLV.CursorUp()
}

func (r *resultModel) pageDown() {
	if r.treeMode {
		r.treeLV.PageDown()
		return
	}
	r.syncFlatRows()
	r.flatLV.PageDown()
}

func (r *resultModel) pageUp() {
	if r.treeMode {
		r.treeLV.PageUp()
		return
	}
	r.syncFlatRows()
	r.flatLV.PageUp()
}

func (r *resultModel) goTop() {
	if r.treeMode {
		r.treeLV.GoTop()
		return
	}
	r.flatLV.GoTop()
}

func (r *resultModel) goBottom() {
	if r.treeMode {
		r.treeLV.GoBottom()
		return
	}
	r.syncFlatRows()
	r.flatLV.GoBottom()
}

func (r *resultModel) historyPrev() {
	r.history.prev()
	r.resetView()
}

func (r *resultModel) historyNext() {
	r.history.next()
	r.resetView()
}

// toggleTreeMode switches between flat and tree display.
func (r *resultModel) toggleTreeMode() {
	r.treeMode = !r.treeMode
	if r.treeMode {
		r.rebuildTree()
	}
	r.flatLV.GoTop()
}

// rebuildTree constructs the tree from the current result group.
func (r *resultModel) rebuildTree() {
	g := r.history.current()
	if g == nil || r.mib == nil {
		r.resultTree = nil
		r.treeLV.SetRows(nil)
		return
	}
	r.resultTree = buildResultTree(g.results, g.walkRootOID, r.mib)
	r.rebuildTreeRows()
}

// rebuildTreeRows re-flattens the tree and updates the list view.
func (r *resultModel) rebuildTreeRows() {
	if r.resultTree == nil {
		r.treeLV.SetRows(nil)
		return
	}
	rows := flattenResultTree(r.resultTree)
	r.treeLV.SetRows(rows)
}

// toggleTreeNode expands/collapses the node at cursor in tree mode.
func (r *resultModel) toggleTreeNode() {
	if !r.treeMode {
		return
	}
	sel := r.treeLV.Selected()
	if sel == nil {
		return
	}
	node := sel.node
	if len(node.children) > 0 {
		node.expanded = !node.expanded
		r.rebuildTreeRows()
	}
}

// collapseTreeNode collapses current node or moves to parent.
func (r *resultModel) collapseTreeNode() {
	if !r.treeMode {
		return
	}
	sel := r.treeLV.Selected()
	if sel == nil {
		return
	}
	node := sel.node
	if node.expanded && len(node.children) > 0 {
		node.expanded = false
		r.rebuildTreeRows()
		return
	}
	// Move to parent - find the row at a lower depth
	curDepth := sel.depth
	cursor := r.treeLV.Cursor()
	for i := cursor - 1; i >= 0; i-- {
		if r.treeLV.Row(i).depth < curDepth {
			r.treeLV.SetCursor(i)
			return
		}
	}
}

// expandTreeNode expands the node at cursor in tree mode.
func (r *resultModel) expandTreeNode() {
	if !r.treeMode {
		return
	}
	sel := r.treeLV.Selected()
	if sel == nil {
		return
	}
	node := sel.node
	if len(node.children) > 0 && !node.expanded {
		node.expanded = true
		r.rebuildTreeRows()
	}
}

// selectedResult returns the snmpResult at the current cursor position (flat mode).
func (r *resultModel) selectedResult() *snmpResult {
	if r.treeMode {
		if sel := r.treeLV.Selected(); sel != nil {
			return sel.node.result
		}
		return nil
	}
	return r.filteredResult(r.flatLV.Cursor())
}

// selectedTreeNode returns the MIB node for the currently selected tree row.
func (r *resultModel) selectedTreeNode() *mib.Node {
	if r.treeMode {
		if sel := r.treeLV.Selected(); sel != nil {
			return sel.node.mibNode
		}
	}
	return nil
}

// clickRow handles a mouse click at the given row index within the data area.
func (r *resultModel) clickRow(row int) {
	if r.treeMode {
		if row >= 0 && row < r.treeLV.Len() {
			r.treeLV.SetCursor(row)
		}
		return
	}
	total := r.totalRows()
	if row >= 0 && row < total {
		r.syncFlatRows()
		r.flatLV.SetCursor(row)
	}
}

// dataOffset returns the scroll offset for the current mode's data rows.
func (r *resultModel) dataOffset() int {
	if r.treeMode {
		return r.treeLV.Offset()
	}
	return r.flatLV.Offset()
}

func (r *resultModel) view(focused bool) string {
	r.focused = focused
	if r.treeMode {
		return r.viewTree(focused)
	}
	return r.viewFlat(focused)
}

func (r *resultModel) viewFlat(focused bool) string {
	g := r.history.current()
	if g == nil {
		return styles.EmptyText.Render("No results yet. Press g to GET selected OID.")
	}

	var b strings.Builder

	// Header line
	header := r.headerLine(g)
	b.WriteString(styles.Header.Info.Render(header))
	b.WriteByte('\n')

	if g.err != nil {
		b.WriteString(styles.Status.ErrorMsg.Render("Error: " + g.err.Error()))
		return b.String()
	}

	total := r.filteredTotalRows()
	if total == 0 {
		if r.filterQuery != "" {
			b.WriteString(styles.EmptyText.Render("(no matches)"))
		} else {
			b.WriteString(styles.EmptyText.Render("(no results)"))
		}
		return b.String()
	}

	// Sync flat list view rows for correct offset/cursor
	r.syncFlatRows()
	cursor := r.flatLV.Cursor()
	offset := r.flatLV.Offset()

	vis := r.flatLV.VisibleRows()
	needScroll := total > vis
	contentW := r.width
	if needScroll {
		contentW = r.width - 1
	}

	// Underline
	b.WriteString(styles.Header.Underline.Render(strings.Repeat("\u2500", min(len(header)+4, contentW))))
	b.WriteByte('\n')

	end := offset + vis
	if end > total {
		end = total
	}

	nameW := 0
	for i := range total {
		res := r.filteredResult(i)
		w := len(res.name)
		if r.showRawOID {
			w += len(res.oid) + 3 // " (oid)"
		}
		if w > nameW {
			nameW = w
		}
	}
	if nameW > contentW/2 {
		nameW = contentW / 2
	}

	for i := offset; i < end; i++ {
		res := r.filteredResult(i)
		name := res.name
		if r.showRawOID {
			name += " " + styles.Subtle.Render("("+res.oid+")")
		}
		nameVisW := lipgloss.Width(name)
		if nameVisW > nameW {
			name = name[:nameW]
			nameVisW = nameW
		}

		typLabel := styles.Label.Render(fmt.Sprintf("%-10s", res.typeName))
		padded := name + strings.Repeat(" ", max(0, nameW-nameVisW))
		valStr := res.value

		// Truncate value to fit
		maxVal := contentW - nameW - 12 - 3
		if maxVal > 0 && lipgloss.Width(valStr) > maxVal {
			valStr = valStr[:max(1, maxVal-1)] + "\u2026"
		}

		var line string
		if i == cursor && r.focused {
			bg := styles.Tree.SelectedBg
			selBg := bg.GetBackground()
			border := styles.Tree.FocusBorder.Background(selBg).Render(BorderThick)
			sp := bg.Render(" ")
			tl := styles.Label.Background(selBg).Render(fmt.Sprintf("%-10s", res.typeName))
			nm := styles.Value.Background(selBg).Render(padded)
			eq := bg.Foreground(styles.Value.GetForeground()).Render(" = " + valStr)
			line = padRightBg(border+sp+tl+sp+nm+eq, contentW, bg)
		} else if i == cursor {
			// Unfocused: dim border, no background highlight
			border := styles.Tree.UnfocusBorder.Render(BorderThick)
			line = padRight(border+" "+typLabel+" "+styles.Value.Render(padded)+" = "+valStr, contentW)
		} else {
			line = "  " + typLabel + " " + styles.Value.Render(padded) + " = " + valStr
		}

		b.WriteString(line)
		if i < end-1 {
			b.WriteByte('\n')
		}
	}

	return attachScrollbar(b.String(), vis, total, vis, offset)
}

func (r *resultModel) viewTree(focused bool) string {
	g := r.history.current()
	if g == nil {
		return styles.EmptyText.Render("No results yet. Press g to GET selected OID.")
	}

	var b strings.Builder

	// Header line
	header := r.headerLine(g)
	b.WriteString(styles.Header.Info.Render(header))
	b.WriteByte('\n')

	if g.err != nil {
		b.WriteString(styles.Status.ErrorMsg.Render("Error: " + g.err.Error()))
		return b.String()
	}

	if r.treeLV.Len() == 0 {
		if len(g.results) == 0 {
			b.WriteString(styles.EmptyText.Render("(no results)"))
		} else {
			b.WriteString(styles.EmptyText.Render("(building tree...)"))
		}
		return b.String()
	}

	// Underline
	b.WriteString(styles.Header.Underline.Render(strings.Repeat("\u2500", min(len(header)+4, r.width))))
	b.WriteByte('\n')

	b.WriteString(r.treeLV.Render(r.renderTreeRowFn))
	return b.String()
}

// renderTreeRowFn is the RenderFunc for result tree rows.
func (r *resultModel) renderTreeRowFn(row resultTreeRow, _ int, selected bool, width int) string {
	node := row.node

	indent := strings.Repeat("  ", row.depth)
	icon := treeIcon(row.hasKids, node.expanded)

	if selected {
		return r.renderSelectedTreeRow(row, node, indent, icon, width)
	}

	var line string
	if node.result != nil {
		// Leaf with result value
		typLabel := styles.Label.Render(fmt.Sprintf("%-10s", node.result.typeName))
		name := node.name
		if r.showRawOID && node.result != nil {
			name += " " + styles.Subtle.Render("("+node.result.oid+")")
		}
		valStr := node.result.value

		// Truncate value
		nameW := lipgloss.Width(name)
		maxVal := width - nameW - 12 - 3 - row.depth*2 - 2
		if maxVal > 0 && lipgloss.Width(valStr) > maxVal {
			valStr = valStr[:max(1, maxVal-1)] + "\u2026"
		}

		line = indent + icon + typLabel + " " + styles.Value.Render(name) + " = " + valStr
	} else {
		// Branch node
		name := node.name
		count := node.resultCount()

		// Kind dot for branch nodes with MIB node reference
		var kindDot string
		if node.mibNode != nil {
			kindDot = kindStyle(node.mibNode.Kind()).Render(IconPending) + " "
		}

		line = indent + icon + kindDot + styles.Value.Render(name) +
			styles.Label.Render(fmt.Sprintf(" (%d)", count))
	}

	return "  " + line
}

// renderSelectedTreeRow renders a result tree row with highlighted background.
func (r *resultModel) renderSelectedTreeRow(row resultTreeRow, node *resultTreeNode, indent, icon string, width int) string {
	if !r.focused {
		// Unfocused: dim border, no background highlight - render like a normal row
		border := styles.Tree.UnfocusBorder.Render(BorderThick)
		var line string
		if node.result != nil {
			typLabel := styles.Label.Render(fmt.Sprintf("%-10s", node.result.typeName))
			name := node.name
			if r.showRawOID && node.result != nil {
				name += " " + styles.Subtle.Render("("+node.result.oid+")")
			}
			valStr := node.result.value
			nameW := lipgloss.Width(name)
			maxVal := width - nameW - 12 - 3 - row.depth*2 - 2
			if maxVal > 0 && lipgloss.Width(valStr) > maxVal {
				valStr = valStr[:max(1, maxVal-1)] + "\u2026"
			}
			line = border + " " + indent + icon + typLabel + " " + styles.Value.Render(name) + " = " + valStr
		} else {
			name := node.name
			count := node.resultCount()
			var kindDot string
			if node.mibNode != nil {
				kindDot = kindStyle(node.mibNode.Kind()).Render(IconPending) + " "
			}
			line = border + " " + indent + icon + kindDot + styles.Value.Render(name) +
				styles.Label.Render(fmt.Sprintf(" (%d)", count))
		}
		return padRight(line, width)
	}

	// Focused: bright border + highlighted background
	bg := styles.Tree.SelectedBg
	selBg := bg.GetBackground()

	border := styles.Tree.FocusBorder.Background(selBg).Render(BorderThick)
	sp := bg.Render(" ")

	var line string
	if node.result != nil {
		// Leaf with result value
		typLabel := styles.Label.Background(selBg).Render(fmt.Sprintf("%-10s", node.result.typeName))
		name := node.name
		if r.showRawOID && node.result != nil {
			name += " " + styles.Subtle.Background(selBg).Render("("+node.result.oid+")")
		}
		valStr := node.result.value

		// Truncate value
		nameW := lipgloss.Width(name)
		maxVal := width - nameW - 12 - 3 - row.depth*2 - 2
		if maxVal > 0 && lipgloss.Width(valStr) > maxVal {
			valStr = valStr[:max(1, maxVal-1)] + "\u2026"
		}

		nameStr := styles.Value.Background(selBg).Render(name)
		eqStr := bg.Foreground(styles.Value.GetForeground()).Render(" = " + valStr)
		line = border + sp + bg.Render(indent+icon) + typLabel + sp + nameStr + eqStr
	} else {
		// Branch node
		name := node.name
		count := node.resultCount()

		var kindDot string
		if node.mibNode != nil {
			kindDot = kindStyle(node.mibNode.Kind()).Background(selBg).Render(IconPending) + sp
		}

		nameStr := styles.Value.Background(selBg).Render(name)
		countStr := styles.Label.Background(selBg).Render(fmt.Sprintf(" (%d)", count))
		line = border + sp + bg.Render(indent+icon) + kindDot + nameStr + countStr
	}

	return padRightBg(line, width, bg)
}

// headerLine builds the header text for the current result group.
func (r *resultModel) headerLine(g *resultGroup) string {
	header := g.label
	if g.op == opWalk {
		header += fmt.Sprintf(" (%d)", len(g.results))
	}
	if r.walkStatus != "" {
		header += "  " + IconLoading + " " + r.walkStatus
	}
	if r.history.len() > 1 {
		header += fmt.Sprintf("  [%d/%d]", r.history.index1(), r.history.len())
	}

	// Mode indicators with key hints
	var modes []string
	if r.treeMode {
		modes = append(modes, "v:tree")
	} else {
		modes = append(modes, "v:flat")
	}
	if r.showRawOID {
		modes = append(modes, "o:oid")
	}
	header += "  " + styles.Label.Render("["+strings.Join(modes, " ")+"]")
	if !r.showRawOID {
		header += " " + styles.StatusText.Render("o:oid")
	}

	return header
}

// activateFilter starts the result filter input.
func (r *resultModel) activateFilter() {
	r.filterInput.SetValue("")
	r.filterQuery = ""
	r.filterIdx = nil
	r.filterInput.Focus()
}

// deactivateFilter closes the result filter input.
func (r *resultModel) deactivateFilter() {
	r.filterInput.Blur()
}

// clearFilter removes the active filter.
func (r *resultModel) clearFilter() {
	r.filterQuery = ""
	r.filterIdx = nil
	r.flatLV.GoTop()
}

// isFiltering returns true if a result filter is active.
func (r *resultModel) isFiltering() bool {
	return r.filterQuery != ""
}

// applyFilter recomputes the filtered index set from the current filter query.
func (r *resultModel) applyFilter() {
	query := strings.ToLower(r.filterInput.Value())
	r.filterQuery = query

	if query == "" {
		r.filterIdx = nil
		return
	}

	g := r.history.current()
	if g == nil {
		r.filterIdx = nil
		return
	}

	r.filterIdx = r.filterIdx[:0:0]
	for i, res := range g.results {
		if strings.Contains(strings.ToLower(res.name), query) ||
			strings.Contains(res.oid, query) ||
			strings.Contains(strings.ToLower(res.value), query) ||
			strings.Contains(strings.ToLower(res.typeName), query) {
			r.filterIdx = append(r.filterIdx, i)
		}
	}

	// Sync flat LV and clamp cursor
	r.syncFlatRows()
}

// filterView renders the result filter input line.
func (r *resultModel) filterView() string {
	line := r.filterInput.View()
	if r.filterQuery != "" {
		line += "  " + styles.Status.SuccessMsg.Render(matchBadge(len(r.filterIdx)))
	}
	return line
}

// filteredTotalRows returns the number of visible rows accounting for filter.
func (r *resultModel) filteredTotalRows() int {
	if r.filterQuery != "" && r.filterIdx != nil {
		return len(r.filterIdx)
	}
	g := r.history.current()
	if g == nil {
		return 0
	}
	return len(g.results)
}

// filteredResult returns the snmpResult at the given filtered index.
func (r *resultModel) filteredResult(i int) *snmpResult {
	g := r.history.current()
	if g == nil {
		return nil
	}
	if r.filterQuery != "" && r.filterIdx != nil {
		if i < 0 || i >= len(r.filterIdx) {
			return nil
		}
		return &g.results[r.filterIdx[i]]
	}
	if i < 0 || i >= len(g.results) {
		return nil
	}
	return &g.results[i]
}
