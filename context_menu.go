package main

import (
	"fmt"
	"image"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/golangsnmp/gomib/mib"
	"github.com/golangsnmp/mibsh/internal/snmp"
	"github.com/muesli/termenv"
)

// Pre-built styles for context menu items, avoiding per-frame allocation.
var (
	ctxMenuSelStyle = lipgloss.NewStyle().
			Background(styles.Tree.SelectedBg.GetBackground()).
			Foreground(styles.Value.GetForeground())
	ctxMenuSelKeyStyle = lipgloss.NewStyle().
				Background(styles.Tree.SelectedBg.GetBackground()).
				Foreground(styles.Value.GetForeground()).
				Bold(true)
	ctxMenuLabelStyle = lipgloss.NewStyle().
				Background(palette.BgLighter).
				Foreground(styles.Value.GetForeground())
	ctxMenuKeyStyle = lipgloss.NewStyle().
			Background(palette.BgLighter).
			Foreground(styles.Value.GetForeground()).
			Bold(true)
	ctxMenuDimStyle = lipgloss.NewStyle().
			Background(palette.BgLighter).
			Foreground(palette.Muted)
)

// contextMenuItem is a single entry in the context menu.
type contextMenuItem struct {
	label   string
	key     string // shortcut hint displayed right-aligned (e.g. "sg")
	enabled bool
	action  func(model) (tea.Model, tea.Cmd)
	isSep   bool // separator line (no action)
}

// contextMenuModel manages a floating right-click context menu.
type contextMenuModel struct {
	items   []contextMenuItem
	cursor  int
	x, y    int // screen position of right-click
	visible bool
}

func (c *contextMenuModel) open(items []contextMenuItem, x, y int) {
	c.items = items
	c.x = x
	c.y = y
	c.visible = true
	// Set cursor to first selectable item
	c.cursor = -1
	for i, item := range items {
		if !item.isSep && item.enabled {
			c.cursor = i
			break
		}
	}
}

func (c *contextMenuModel) close() {
	c.visible = false
	c.items = nil
	c.cursor = -1
}

func (c *contextMenuModel) cursorDown() {
	for i := c.cursor + 1; i < len(c.items); i++ {
		if !c.items[i].isSep && c.items[i].enabled {
			c.cursor = i
			return
		}
	}
}

func (c *contextMenuModel) cursorUp() {
	for i := c.cursor - 1; i >= 0; i-- {
		if !c.items[i].isSep && c.items[i].enabled {
			c.cursor = i
			return
		}
	}
}

func (c *contextMenuModel) selectedItem() *contextMenuItem {
	if c.cursor < 0 || c.cursor >= len(c.items) {
		return nil
	}
	item := &c.items[c.cursor]
	if item.isSep || !item.enabled {
		return nil
	}
	return item
}

// menuRect computes the rendered bounding rectangle for the menu,
// clamped to the screen area.
func (c *contextMenuModel) menuRect(area image.Rectangle) image.Rectangle {
	content := c.buildContent()
	if content == "" {
		return image.Rectangle{}
	}
	content = padContentBg(content, palette.BgLighter)
	box := styles.Tooltip.Box.Render(content)
	w := lipgloss.Width(box)
	h := lipgloss.Height(box)

	x, y := clampRect(c.x, c.y, w, h, area)

	return image.Rect(x, y, x+w, y+h)
}

// containsPoint returns true if (px, py) is inside the rendered menu area.
func (c *contextMenuModel) containsPoint(px, py int, area image.Rectangle) bool {
	return image.Pt(px, py).In(c.menuRect(area))
}

// itemAtY maps a screen Y coordinate to an item index within the menu.
// Returns -1 if outside items. The box has a 1-row border on top.
func (c *contextMenuModel) itemAtY(screenY int, area image.Rectangle) int {
	rect := c.menuRect(area)
	// Tooltip.Box uses RoundedBorder (1 row) + Padding(0, 1) = 1 row top border
	contentY := screenY - rect.Min.Y - 1 // 1 for border
	if contentY < 0 || contentY >= len(c.items) {
		return -1
	}
	return contentY
}

func (c *contextMenuModel) draw(canvas uv.ScreenBuffer, area image.Rectangle) {
	if !c.visible || len(c.items) == 0 {
		return
	}

	content := c.buildContent()
	if content == "" {
		return
	}

	content = padContentBg(content, palette.BgLighter)
	box := styles.Tooltip.Box.Render(content)
	rect := c.menuRect(area)
	uv.NewStyledString(box).Draw(canvas, rect)
}

func (c *contextMenuModel) buildContent() string {
	if len(c.items) == 0 {
		return ""
	}

	bg := palette.BgLighter

	// Compute max label width and max key width for alignment
	maxLabel := 0
	maxKey := 0
	for _, item := range c.items {
		if item.isSep {
			continue
		}
		if len(item.label) > maxLabel {
			maxLabel = len(item.label)
		}
		if len(item.key) > maxKey {
			maxKey = len(item.key)
		}
	}

	// Gap between label and key
	gap := 2
	menuW := maxLabel + gap + maxKey

	var b strings.Builder
	for i, item := range c.items {
		if i > 0 {
			b.WriteByte('\n')
		}

		if item.isSep {
			sep := styles.Separator.Background(bg).Render(strings.Repeat("\u2500", menuW))
			b.WriteString(sep)
			continue
		}

		isSelected := i == c.cursor

		// Build the line: label + padding + right-aligned key
		labelPad := maxLabel - len(item.label)
		keyPad := maxKey - len(item.key)
		line := item.label + strings.Repeat(" ", labelPad+gap+keyPad)

		if isSelected && item.enabled {
			text := ctxMenuSelStyle.Render(line) + ctxMenuSelKeyStyle.Render(item.key)
			b.WriteString(text)
		} else if item.enabled {
			text := ctxMenuLabelStyle.Render(line) + ctxMenuKeyStyle.Render(item.key)
			b.WriteString(text)
		} else {
			text := ctxMenuDimStyle.Render(line + item.key)
			b.WriteString(text)
		}
	}

	return b.String()
}

// --- Menu item builders ---

func contextSep() contextMenuItem {
	return contextMenuItem{isSep: true}
}

func treeMenuItems(m model) []contextMenuItem {
	node := m.tree.selectedNode()
	connected := m.snmp.IsConnected()
	idle := m.walk == nil
	hasOID := node != nil && node.OID() != nil
	longOID := hasOID && len(node.OID()) >= 2

	snmpReady := connected && idle && longOID

	// Check if this is a table node
	isTable := false
	if node != nil {
		if obj := node.Object(); obj != nil {
			tbl, _ := resolveTable(obj, node.Kind())
			isTable = tbl != nil
		}
	}

	hasXrefs := false
	if node != nil {
		hasXrefs = len(m.detail.xrefs[node.Name()]) > 0
	}

	hasKids := false
	isExpanded := false
	if node != nil {
		hasKids = len(node.Children()) > 0
		if hasOID {
			isExpanded = m.tree.expanded[node.OID().String()]
		}
	}

	items := []contextMenuItem{
		{label: "GET", key: "sg", enabled: snmpReady, action: func(m model) (tea.Model, tea.Cmd) {
			return m.snmpGet()
		}},
		{label: "GETNEXT", key: "sn", enabled: snmpReady, action: func(m model) (tea.Model, tea.Cmd) {
			return m.snmpGetNext()
		}},
		{label: "WALK", key: "sw", enabled: snmpReady, action: func(m model) (tea.Model, tea.Cmd) {
			return m.snmpWalk()
		}},
		{label: "TABLE", key: "st", enabled: snmpReady && isTable, action: func(m model) (tea.Model, tea.Cmd) {
			return m.snmpTableData()
		}},
		contextSep(),
		{label: "Copy OID", key: "y", enabled: hasOID, action: func(m model) (tea.Model, tea.Cmd) {
			if n := m.tree.selectedNode(); n != nil {
				return m, copyToClipboard(n)
			}
			return m, nil
		}},
		{label: "Cross-references", key: "x", enabled: hasXrefs, action: func(m model) (tea.Model, tea.Cmd) {
			if n := m.tree.selectedNode(); n != nil {
				if refs := m.detail.xrefs[n.Name()]; len(refs) > 0 {
					m.xrefPicker.activate(n.Name(), refs)
					m.focus = focusXref
				}
			}
			return m, nil
		}},
		contextSep(),
	}

	if hasKids {
		if isExpanded {
			items = append(items, contextMenuItem{
				label: "Collapse All", key: "", enabled: true,
				action: func(m model) (tea.Model, tea.Cmd) {
					if n := m.tree.selectedNode(); n != nil {
						collapseSubtree(&m.tree, n)
						m.tree.rebuild()
						m.syncSelection()
					}
					return m, nil
				},
			})
		} else {
			items = append(items, contextMenuItem{
				label: "Expand All", key: "", enabled: true,
				action: func(m model) (tea.Model, tea.Cmd) {
					if n := m.tree.selectedNode(); n != nil {
						expandSubtree(&m.tree, n)
						m.tree.rebuild()
						m.syncSelection()
					}
					return m, nil
				},
			})
		}
	}

	return items
}

func resultMenuItems(m model) []contextMenuItem {
	connected := m.snmp.IsConnected()
	idle := m.walk == nil
	res := m.results.selectedResult()
	hasResult := res != nil
	hasOID := hasResult && res.OID != ""

	snmpReady := connected && idle && hasOID

	// Check if we can resolve the MIB node from this result
	canJump := false
	if hasOID {
		oid, err := mib.ParseOID(res.OID)
		if err == nil {
			canJump = m.mib.LongestPrefixByOID(oid) != nil
		}
	}

	return []contextMenuItem{
		{label: "GET", key: "sg", enabled: snmpReady, action: withSelectedResult(func(m model, r *snmp.Result) (tea.Model, tea.Cmd) {
			if ret, retCmd, ok := m.requireConnectedIdle(); !ok {
				return ret, retCmd
			}
			return m, snmp.GetCmd(m.snmp, []string{r.OID})
		})},
		{label: "GETNEXT", key: "sn", enabled: snmpReady, action: withSelectedResult(func(m model, r *snmp.Result) (tea.Model, tea.Cmd) {
			if ret, retCmd, ok := m.requireConnectedIdle(); !ok {
				return ret, retCmd
			}
			return m, snmp.GetNextCmd(m.snmp, r.OID)
		})},
		{label: "WALK", key: "sw", enabled: snmpReady, action: withSelectedResult(func(m model, r *snmp.Result) (tea.Model, tea.Cmd) {
			if ret, retCmd, ok := m.requireConnectedIdle(); !ok {
				return ret, retCmd
			}
			return m.startQueryWalk(r.OID)
		})},
		contextSep(),
		{label: "Copy OID", key: "", enabled: hasOID, action: withSelectedResult(func(m model, r *snmp.Result) (tea.Model, tea.Cmd) {
			return m, copyText(r.OID)
		})},
		{label: "Copy Value", key: "", enabled: hasResult, action: withSelectedResult(func(m model, r *snmp.Result) (tea.Model, tea.Cmd) {
			return m, copyText(r.Value)
		})},
		contextSep(),
		{label: "Jump to Tree", key: "enter", enabled: canJump, action: withSelectedResult(func(m model, r *snmp.Result) (tea.Model, tea.Cmd) {
			m.crossRefResultByOID(r.OID)
			return m, nil
		})},
	}
}

func detailMenuItems(m model) []contextMenuItem {
	node := m.detail.node
	hasOID := node != nil && node.OID() != nil
	hasXrefs := false
	if node != nil {
		hasXrefs = len(m.detail.xrefs[node.Name()]) > 0
	}

	return []contextMenuItem{
		{label: "Copy OID", key: "y", enabled: hasOID, action: func(m model) (tea.Model, tea.Cmd) {
			if n := m.detail.node; n != nil {
				return m, copyToClipboard(n)
			}
			return m, nil
		}},
		{label: "Cross-references", key: "x", enabled: hasXrefs, action: func(m model) (tea.Model, tea.Cmd) {
			if n := m.detail.node; n != nil {
				if refs := m.detail.xrefs[n.Name()]; len(refs) > 0 {
					m.xrefPicker.activate(n.Name(), refs)
					m.focus = focusXref
				}
			}
			return m, nil
		}},
	}
}

func tableDataMenuItems(m model) []contextMenuItem {
	hasRow := m.tableData.cursor >= 0 && m.tableData.cursor < len(m.tableData.rows)

	return []contextMenuItem{
		{label: "Copy Row", key: "", enabled: hasRow, action: func(m model) (tea.Model, tea.Cmd) {
			if m.tableData.cursor < 0 || m.tableData.cursor >= len(m.tableData.rows) {
				return m, nil
			}
			row := m.tableData.rows[m.tableData.cursor]
			var parts []string
			for i, cell := range row {
				col := ""
				if i < len(m.tableData.columns) {
					col = m.tableData.columns[i]
				}
				parts = append(parts, fmt.Sprintf("%s=%s", col, cell))
			}
			return m, copyText(strings.Join(parts, " "))
		}},
	}
}

// withSelectedResult wraps a context menu action that needs the currently
// selected result. If no result is selected, it returns (m, nil).
func withSelectedResult(fn func(model, *snmp.Result) (tea.Model, tea.Cmd)) func(model) (tea.Model, tea.Cmd) {
	return func(m model) (tea.Model, tea.Cmd) {
		r := m.results.selectedResult()
		if r == nil {
			return m, nil
		}
		return fn(m, r)
	}
}

// --- Helper functions ---

// expandSubtree recursively expands a node and all descendants with children.
func expandSubtree(t *treeModel, node *mib.Node) {
	if len(node.Children()) == 0 {
		return
	}
	if oid := node.OID(); oid != nil {
		t.expanded[oid.String()] = true
	}
	for _, child := range node.Children() {
		expandSubtree(t, child)
	}
}

// collapseSubtree recursively collapses a node and all descendants.
func collapseSubtree(t *treeModel, node *mib.Node) {
	if oid := node.OID(); oid != nil {
		delete(t.expanded, oid.String())
	}
	for _, child := range node.Children() {
		collapseSubtree(t, child)
	}
}

// copyText copies arbitrary text to the clipboard and returns a status message.
func copyText(text string) tea.Cmd {
	return func() tea.Msg {
		termenv.Copy(text)
		_ = clipboard.WriteAll(text)
		return statusMsg{typ: statusSuccess, text: "Copied: " + text}
	}
}
