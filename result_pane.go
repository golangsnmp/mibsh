package main

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
	"github.com/golangsnmp/gomib/mib"
	"github.com/golangsnmp/mibsh/internal/snmp"
)

// Layout constants for result pane rendering.
const (
	typeColumnWidth = 10 // width of the type label column (e.g. "INTEGER   ")
	typeColumnPad   = 12 // typeColumnWidth + 2 surrounding spaces
	valueEqSepWidth = 3  // width of " = " separator
	treeIndentWidth = 2  // spaces per tree depth level
	treeLeafGutter  = 2  // leading spaces for non-selected tree leaf rows
)

// truncateValue truncates a string to fit within maxWidth using byte length,
// appending a unicode ellipsis if truncated. This is suitable for value strings
// where visual width roughly matches byte length.
func truncateValue(s string, maxWidth int) string {
	if maxWidth <= 0 || lipgloss.Width(s) <= maxWidth {
		return s
	}
	return s[:max(1, maxWidth-1)] + "\u2026"
}

// resultModel displays SNMP operation results in the bottom-right pane.
type resultModel struct {
	history    snmp.ResultHistory
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

func (r *resultModel) setSize(width, height int) {
	r.width = width
	r.height = height
	r.flatLV.SetSize(width, height)
	r.treeLV.SetSize(width, height)
}

func (r *resultModel) addGroup(g snmp.ResultGroup) {
	r.history.Add(g)
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

func (r *resultModel) appendResults(results []snmp.Result) {
	g := r.history.Current()
	if g == nil {
		return
	}
	g.Results = append(g.Results, results...)

	// Update tree if in tree mode
	if r.treeMode && r.resultTree != nil && r.mib != nil {
		for i := range results {
			insertResultIntoTree(r.resultTree, &g.Results[len(g.Results)-len(results)+i], g.WalkRootOID, r.mib)
		}
		r.rebuildTreeRows()
	}
}

// listNav is the subset of ListView methods shared by both tree and flat modes.
type listNav interface {
	CursorDown()
	CursorUp()
	PageDown()
	PageUp()
	GoTop()
	GoBottom()
	Len() int
	Offset() int
	SetCursor(int)
}

// activeLV returns the active list navigator for the current display mode.
// When syncFlat is true and in flat mode, syncFlatRows is called first to
// ensure the flat ListView row count matches the filtered result count.
func (r *resultModel) activeLV(syncFlat bool) listNav {
	if r.treeMode {
		return &r.treeLV
	}
	if syncFlat {
		r.syncFlatRows()
	}
	return &r.flatLV
}

// syncFlatRows updates the flat ListView row count to match the current
// filtered total, preserving cursor position.
func (r *resultModel) syncFlatRows() {
	total := r.filteredTotalRows()
	rows := make([]struct{}, total)
	r.flatLV.SetRows(rows)
}

func (r *resultModel) cursorDown() { r.activeLV(true).CursorDown() }
func (r *resultModel) cursorUp()   { r.activeLV(true).CursorUp() }
func (r *resultModel) pageDown()   { r.activeLV(true).PageDown() }
func (r *resultModel) pageUp()     { r.activeLV(true).PageUp() }
func (r *resultModel) goTop()      { r.activeLV(false).GoTop() }
func (r *resultModel) goBottom()   { r.activeLV(true).GoBottom() }

func (r *resultModel) historyPrev() {
	r.history.Prev()
	r.resetView()
}

func (r *resultModel) historyNext() {
	r.history.Next()
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
	g := r.history.Current()
	if g == nil || r.mib == nil {
		r.resultTree = nil
		r.treeLV.SetRows(nil)
		return
	}
	r.resultTree = buildResultTree(g.Results, g.WalkRootOID, r.mib)
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

// selectedResult returns the snmp.Result at the current cursor position (flat mode).
func (r *resultModel) selectedResult() *snmp.Result {
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
	lv := r.activeLV(true)
	if row >= 0 && row < lv.Len() {
		lv.SetCursor(row)
	}
}

// dataOffset returns the scroll offset for the current mode's data rows.
func (r *resultModel) dataOffset() int {
	return r.activeLV(false).Offset()
}

func (r *resultModel) view(focused bool) string {
	r.focused = focused
	if r.treeMode {
		return r.viewTree(focused)
	}
	return r.viewFlat(focused)
}

func (r *resultModel) viewFlat(focused bool) string {
	g := r.history.Current()
	if g == nil {
		return styles.EmptyText.Render("No results yet. Press g to GET selected OID.")
	}

	var b strings.Builder

	// Header line
	header := r.headerLine(g)
	b.WriteString(styles.Header.Info.Render(header))
	b.WriteByte('\n')

	if g.Err != nil {
		b.WriteString(styles.Status.ErrorMsg.Render("Error: " + g.Err.Error()))
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
		w := len(res.Name)
		if r.showRawOID {
			w += len(res.OID) + 3 // " (oid)"
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
		name := res.Name
		if r.showRawOID {
			name += " " + styles.Subtle.Render("("+res.OID+")")
		}
		nameVisW := lipgloss.Width(name)
		if nameVisW > nameW {
			name = name[:nameW]
			nameVisW = nameW
		}

		typLabel := styles.Label.Render(fmt.Sprintf("%-*s", typeColumnWidth, res.TypeName))
		padded := name + strings.Repeat(" ", max(0, nameW-nameVisW))
		valStr := res.Value

		// Truncate value to fit
		maxVal := contentW - nameW - typeColumnPad - valueEqSepWidth
		valStr = truncateValue(valStr, maxVal)

		var line string
		if i == cursor && r.focused {
			bg := styles.Tree.SelectedBg
			selBg := bg.GetBackground()
			border := styles.Tree.FocusBorder.Background(selBg).Render(BorderThick)
			sp := bg.Render(" ")
			tl := styles.Label.Background(selBg).Render(fmt.Sprintf("%-*s", typeColumnWidth, res.TypeName))
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
	g := r.history.Current()
	if g == nil {
		return styles.EmptyText.Render("No results yet. Press g to GET selected OID.")
	}

	var b strings.Builder

	// Header line
	header := r.headerLine(g)
	b.WriteString(styles.Header.Info.Render(header))
	b.WriteByte('\n')

	if g.Err != nil {
		b.WriteString(styles.Status.ErrorMsg.Render("Error: " + g.Err.Error()))
		return b.String()
	}

	if r.treeLV.Len() == 0 {
		if len(g.Results) == 0 {
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

// treeLeafParts holds the raw data for rendering a leaf tree row.
type treeLeafParts struct {
	typeName string // formatted type name, e.g. "INTEGER   "
	name     string // node name (without OID suffix)
	oid      string // raw OID string, empty if showRawOID is off
	value    string // truncated display value
}

// treeLeaf computes the content parts for a leaf (result-bearing) tree row.
func (r *resultModel) treeLeaf(node *resultTreeNode, depth, width int) treeLeafParts {
	p := treeLeafParts{
		typeName: fmt.Sprintf("%-*s", typeColumnWidth, node.result.TypeName),
		name:     node.name,
		value:    node.result.Value,
	}
	if r.showRawOID {
		p.oid = node.result.OID
	}

	// Compute visual width of name (including OID suffix if present) for truncation.
	nameW := len(p.name)
	if p.oid != "" {
		nameW += 1 + 1 + len(p.oid) + 1 // " " + "(" + oid + ")"
	}

	maxVal := width - nameW - typeColumnPad - valueEqSepWidth - depth*treeIndentWidth - treeLeafGutter
	p.value = truncateValue(p.value, maxVal)
	return p
}

// treeBranchParts holds the raw data for rendering a branch tree row.
type treeBranchParts struct {
	name    string
	count   int
	mibNode *mib.Node // may be nil; used for kind dot
}

// treeBranch computes the content parts for a branch (non-leaf) tree row.
func treeBranch(node *resultTreeNode) treeBranchParts {
	return treeBranchParts{
		name:    node.name,
		count:   node.resultCount(),
		mibNode: node.mibNode,
	}
}

// renderTreeRowFn is the RenderFunc for result tree rows.
func (r *resultModel) renderTreeRowFn(row resultTreeRow, _ int, selected bool, width int) string {
	node := row.node
	indent := strings.Repeat("  ", row.depth)
	icon := treeIcon(row.hasKids, node.expanded)

	if selected {
		return r.renderSelectedTreeRow(row, indent, icon, width)
	}

	var line string
	if node.result != nil {
		p := r.treeLeaf(node, row.depth, width)
		name := p.name
		if p.oid != "" {
			name += " " + styles.Subtle.Render("("+p.oid+")")
		}
		typLabel := styles.Label.Render(p.typeName)
		line = indent + icon + typLabel + " " + styles.Value.Render(name) + " = " + p.value
	} else {
		bp := treeBranch(node)
		var kindDot string
		if bp.mibNode != nil {
			kindDot = kindStyle(bp.mibNode.Kind()).Render(IconPending) + " "
		}
		line = indent + icon + kindDot + styles.Value.Render(bp.name) +
			styles.Label.Render(fmt.Sprintf(" (%d)", bp.count))
	}

	return "  " + line
}

// renderSelectedTreeRow renders a result tree row with highlighted background.
func (r *resultModel) renderSelectedTreeRow(row resultTreeRow, indent, icon string, width int) string {
	node := row.node

	if !r.focused {
		border := styles.Tree.UnfocusBorder.Render(BorderThick)
		var line string
		if node.result != nil {
			p := r.treeLeaf(node, row.depth, width)
			name := p.name
			if p.oid != "" {
				name += " " + styles.Subtle.Render("("+p.oid+")")
			}
			typLabel := styles.Label.Render(p.typeName)
			line = border + " " + indent + icon + typLabel + " " + styles.Value.Render(name) + " = " + p.value
		} else {
			bp := treeBranch(node)
			var kindDot string
			if bp.mibNode != nil {
				kindDot = kindStyle(bp.mibNode.Kind()).Render(IconPending) + " "
			}
			line = border + " " + indent + icon + kindDot + styles.Value.Render(bp.name) +
				styles.Label.Render(fmt.Sprintf(" (%d)", bp.count))
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
		p := r.treeLeaf(node, row.depth, width)
		name := p.name
		if p.oid != "" {
			name += " " + styles.Subtle.Background(selBg).Render("("+p.oid+")")
		}
		typLabel := styles.Label.Background(selBg).Render(p.typeName)
		nameStr := styles.Value.Background(selBg).Render(name)
		eqStr := bg.Foreground(styles.Value.GetForeground()).Render(" = " + p.value)
		line = border + sp + bg.Render(indent+icon) + typLabel + sp + nameStr + eqStr
	} else {
		bp := treeBranch(node)
		var kindDot string
		if bp.mibNode != nil {
			kindDot = kindStyle(bp.mibNode.Kind()).Background(selBg).Render(IconPending) + sp
		}
		nameStr := styles.Value.Background(selBg).Render(bp.name)
		countStr := styles.Label.Background(selBg).Render(fmt.Sprintf(" (%d)", bp.count))
		line = border + sp + bg.Render(indent+icon) + kindDot + nameStr + countStr
	}

	return padRightBg(line, width, bg)
}

// headerLine builds the header text for the current result group.
func (r *resultModel) headerLine(g *snmp.ResultGroup) string {
	header := g.Label
	if g.Op == snmp.OpWalk {
		header += fmt.Sprintf(" (%d)", len(g.Results))
	}
	if r.walkStatus != "" {
		header += "  " + IconLoading + " " + r.walkStatus
	}
	if r.history.Len() > 1 {
		header += fmt.Sprintf("  [%d/%d]", r.history.Index1(), r.history.Len())
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

	g := r.history.Current()
	if g == nil {
		r.filterIdx = nil
		return
	}

	r.filterIdx = r.filterIdx[:0:0]
	for i, res := range g.Results {
		if strings.Contains(strings.ToLower(res.Name), query) ||
			strings.Contains(res.OID, query) ||
			strings.Contains(strings.ToLower(res.Value), query) ||
			strings.Contains(strings.ToLower(res.TypeName), query) {
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
	g := r.history.Current()
	if g == nil {
		return 0
	}
	return len(g.Results)
}

// filteredResult returns the snmp.Result at the given filtered index.
func (r *resultModel) filteredResult(i int) *snmp.Result {
	g := r.history.Current()
	if g == nil {
		return nil
	}
	if r.filterQuery != "" && r.filterIdx != nil {
		if i < 0 || i >= len(r.filterIdx) {
			return nil
		}
		return &g.Results[r.filterIdx[i]]
	}
	if i < 0 || i >= len(g.Results) {
		return nil
	}
	return &g.Results[i]
}
