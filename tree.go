package main

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/golangsnmp/gomib/mib"
)

// treeRow is a single visible row in the flattened tree.
type treeRow struct {
	node     *mib.Node
	depth    int
	expanded bool
	hasKids  bool
	matched  bool // true if this node directly matches the filter (vs ancestor-only)
}

// treeModel is the tree browser component.
type treeModel struct {
	root         *mib.Node
	expanded     map[string]bool // keyed by OID string
	lv           ListView[treeRow]
	filter       *celFilter      // shared with filterBarModel
	filterMatch  map[string]bool // OID -> in filtered set (match or ancestor)
	filterDirect map[string]bool // OID -> directly matches expression
	filterActive bool
	focused      bool // set during view() to control border style
}

func newTreeModel(root *mib.Node) treeModel {
	t := treeModel{
		root:     root,
		expanded: make(map[string]bool),
		lv:       NewListView[treeRow](0),
	}
	// Auto-expand the default path: iso.org.dod.internet
	t.autoExpand()
	t.rebuild()
	return t
}

// autoExpand expands iso(1).org(3).dod(6).internet(1) so the user starts
// with something visible.
func (t *treeModel) autoExpand() {
	node := t.root
	path := []uint32{1, 3, 6, 1}
	for _, arc := range path {
		child := node.Child(arc)
		if child == nil {
			break
		}
		oid := child.OID().String()
		t.expanded[oid] = true
		node = child
	}
}

// rebuild flattens the tree according to current expanded state.
func (t *treeModel) rebuild() {
	if t.filterActive && t.filter != nil && t.filter.program != nil {
		t.rebuildFiltered()
		return
	}
	t.filterMatch = nil
	t.filterDirect = nil
	rows := t.lv.Rows()[:0]
	for _, child := range t.root.Children() {
		rows = t.flatten(child, 0, rows)
	}
	t.lv.SetRows(rows)
}

// rebuildFiltered does a two-pass filter: first evaluate CEL on every node to
// find matches and their ancestors, then flatten only matching/ancestor nodes.
func (t *treeModel) rebuildFiltered() {
	t.filterMatch = make(map[string]bool)
	t.filterDirect = make(map[string]bool)
	t.filter.matchCount = 0

	// Pass 1: walk entire tree, evaluate filter, mark matches + ancestors
	for _, child := range t.root.Children() {
		t.matchSubtree(child)
	}

	// Pass 2: flatten only nodes in filterMatch, auto-expanding paths
	rows := t.lv.Rows()[:0]
	for _, child := range t.root.Children() {
		rows = t.flattenFiltered(child, 0, rows)
	}
	t.lv.SetRows(rows)
}

// matchSubtree recursively evaluates the filter on a subtree. Returns true if
// this node or any descendant matches, which causes ancestors to be included.
func (t *treeModel) matchSubtree(node *mib.Node) bool {
	oid := node.OID().String()
	directMatch := t.filter.eval(node)
	if directMatch {
		t.filterDirect[oid] = true
		t.filter.matchCount++
	}

	anyChildMatch := false
	for _, child := range node.Children() {
		if t.matchSubtree(child) {
			anyChildMatch = true
		}
	}

	if directMatch || anyChildMatch {
		t.filterMatch[oid] = true
		return true
	}
	return false
}

// flattenFiltered flattens only nodes present in the filter match set.
func (t *treeModel) flattenFiltered(node *mib.Node, depth int, rows []treeRow) []treeRow {
	oid := node.OID().String()
	if !t.filterMatch[oid] {
		return rows
	}

	kids := node.Children()
	// Check if any children are in the match set
	hasVisibleKids := false
	for _, child := range kids {
		if t.filterMatch[child.OID().String()] {
			hasVisibleKids = true
			break
		}
	}

	rows = append(rows, treeRow{
		node:     node,
		depth:    depth,
		expanded: hasVisibleKids,
		hasKids:  hasVisibleKids,
		matched:  t.filterDirect[oid],
	})

	if hasVisibleKids {
		for _, child := range kids {
			rows = t.flattenFiltered(child, depth+1, rows)
		}
	}
	return rows
}

func (t *treeModel) flatten(node *mib.Node, depth int, rows []treeRow) []treeRow {
	oid := node.OID().String()
	kids := node.Children()
	hasKids := len(kids) > 0
	exp := t.expanded[oid]

	rows = append(rows, treeRow{
		node:     node,
		depth:    depth,
		expanded: exp,
		hasKids:  hasKids,
	})

	if exp && hasKids {
		for _, child := range kids {
			rows = t.flatten(child, depth+1, rows)
		}
	}
	return rows
}

func (t *treeModel) selectedNode() *mib.Node {
	if sel := t.lv.Selected(); sel != nil {
		return sel.node
	}
	return nil
}

func (t *treeModel) toggle() {
	sel := t.lv.Selected()
	if sel == nil || !sel.hasKids {
		return
	}
	oid := sel.node.OID().String()
	t.expanded[oid] = !t.expanded[oid]
	t.rebuild()
}

func (t *treeModel) collapse() {
	sel := t.lv.Selected()
	if sel == nil {
		return
	}
	oid := sel.node.OID().String()
	if t.expanded[oid] {
		t.expanded[oid] = false
		t.rebuild()
		return
	}
	// Move to parent
	t.moveToParent()
}

func (t *treeModel) moveToParent() {
	sel := t.lv.Selected()
	if sel == nil {
		return
	}
	parent := sel.node.Parent()
	if parent == nil || parent == t.root {
		return
	}
	parentOID := parent.OID().String()
	for i, r := range t.lv.Rows() {
		if r.node.OID().String() == parentOID {
			t.lv.SetCursor(i)
			return
		}
	}
}

func (t *treeModel) cursorDown()    { t.lv.CursorDown() }
func (t *treeModel) cursorUp()      { t.lv.CursorUp() }
func (t *treeModel) cursorBy(n int) { t.lv.CursorBy(n) }
func (t *treeModel) pageDown()      { t.lv.PageDown() }
func (t *treeModel) pageUp()        { t.lv.PageUp() }
func (t *treeModel) goTop()         { t.lv.GoTop() }
func (t *treeModel) goBottom()      { t.lv.GoBottom() }

func (t *treeModel) setSize(width, height int) {
	t.lv.SetSize(width, height)
}

// jumpToNode expands all ancestors and sets cursor to the given node.
func (t *treeModel) jumpToNode(node *mib.Node) {
	// Expand all ancestors
	ancestors := collectAncestors(node, t.root)
	for _, a := range ancestors {
		oid := a.OID().String()
		if len(a.Children()) > 0 {
			t.expanded[oid] = true
		}
	}
	t.rebuild()

	// Find and set cursor
	targetOID := node.OID().String()
	for i, row := range t.lv.Rows() {
		if row.node.OID().String() == targetOID {
			t.lv.SetCursor(i)
			return
		}
	}
}

func collectAncestors(node *mib.Node, root *mib.Node) []*mib.Node {
	var ancestors []*mib.Node
	for n := node.Parent(); n != nil && n != root; n = n.Parent() {
		ancestors = append(ancestors, n)
	}
	// Reverse so we expand from root downward
	slices.Reverse(ancestors)
	return ancestors
}

func (t *treeModel) view(focused bool) string {
	t.focused = focused

	if t.lv.Len() == 0 {
		return "(empty tree)"
	}

	return t.lv.Render(t.renderRowFn)
}

// renderRowFn is the RenderFunc for treeModel.
func (t *treeModel) renderRowFn(row treeRow, _ int, selected bool, width int) string {
	if selected {
		return t.renderSelectedRow(row, width)
	}
	line := t.renderRow(row)
	return t.styleRow(row, line)
}

// renderSelectedRow renders the cursor row with thick left border, kind dot,
// and highlighted background.
func (t *treeModel) renderSelectedRow(row treeRow, width int) string {
	indent := strings.Repeat("  ", row.depth)
	icon := treeIcon(row.hasKids, row.expanded)
	label := nodeLabel(row.node)

	if !t.focused {
		// Unfocused: dim border, no background highlight
		border := styles.Tree.UnfocusBorder.Render(BorderThick)
		text := indent + icon + label + fmt.Sprintf("(%d)", row.node.Arc())
		line := border + " " + kindStyle(row.node.Kind()).Render(text)
		return padRight(line, width)
	}

	// Focused: bright border + highlighted background
	bg := styles.Tree.SelectedBg

	// Each segment gets the selection background so ANSI resets between
	// styled segments don't kill the highlight.
	border := styles.Tree.FocusBorder.Background(bg.GetBackground()).Render(BorderThick)
	text := bg.Foreground(styles.Value.GetForeground()).Render(indent + icon + label + fmt.Sprintf("(%d)", row.node.Arc()))

	line := border + bg.Render(" ") + text

	return padRightBg(line, width, bg)
}

func (t *treeModel) renderRow(row treeRow) string {
	indent := strings.Repeat("  ", row.depth)
	icon := treeIcon(row.hasKids, row.expanded)
	label := nodeLabel(row.node)

	// Non-selected: 2-char gutter (matching border width) + text
	base := "  " + indent + icon + label + fmt.Sprintf("(%d)", row.node.Arc())

	return base
}

// styleRow applies kind-based color and status dimming to a tree row.
func (t *treeModel) styleRow(row treeRow, line string) string {
	// Dim ancestor-only rows when filter is active
	if t.filterActive && !row.matched {
		return styles.Tree.Deprecated.Render(line)
	}
	status := nodeStatus(row.node)
	switch status {
	case mib.StatusDeprecated:
		return styles.Tree.Deprecated.Render(line)
	case mib.StatusObsolete:
		return styles.Tree.Obsolete.Render(line)
	default:
		return kindStyle(row.node.Kind()).Render(line)
	}
}

func padRight(s string, width int) string {
	n := lipgloss.Width(s)
	if n >= width {
		return s
	}
	return s + strings.Repeat(" ", width-n)
}

// padRightBg pads the string to width using spaces styled with the given style
// (typically a background-only style), so the highlight extends to the edge.
func padRightBg(s string, width int, style lipgloss.Style) string {
	n := lipgloss.Width(s)
	if n >= width {
		return s
	}
	return s + style.Render(strings.Repeat(" ", width-n))
}
