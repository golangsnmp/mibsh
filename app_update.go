package main

import (
	"image"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/golangsnmp/gomib/mib"
	"github.com/golangsnmp/mibsh/internal/snmp"
)

// mouseScrollLines is the number of rows/lines to move per mouse wheel tick.
const mouseScrollLines = 3

// statusDisplayDuration is how long status messages remain visible before clearing.
const statusDisplayDuration = 5 * time.Second

// resultHeaderLines is the number of header lines (label + underline) before result data rows.
const resultHeaderLines = 2

// tableDataHeaderLines is the number of header lines (label + column headers + separator)
// before table data rows.
const tableDataHeaderLines = 3

// setStatus sets the current status message without returning a tea.Cmd.
// Use this when you need to set status in the middle of a flow that returns
// its own command. The caller is responsible for calling clearStatusAfter.
func (m *model) setStatus(typ statusType, text string) {
	m.status.current = &statusMsg{typ: typ, text: text}
}

// setStatusReturn sets a status message and returns the model with a clear-after command.
func (m model) setStatusReturn(typ statusType, text string) (tea.Model, tea.Cmd) {
	m.setStatus(typ, text)
	return m, clearStatusAfter(statusDisplayDuration)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateLayout()
		return m, nil

	case tea.MouseClickMsg:
		m.pendingChord = ""
		if m.overlay.kind == overlayConnect && m.dialog != nil {
			return m.handleDialogClick(msg)
		}
		if !m.overlay.isDialog() {
			m.tooltip.hide()
			return m.handleMouseClick(msg)
		}
		return m, nil

	case tea.MouseWheelMsg:
		m.pendingChord = ""
		if !m.overlay.isDialog() {
			m.tooltip.hide()
			return m.handleMouseWheel(msg)
		}
		return m, nil

	case tea.MouseMotionMsg:
		return m.handleMouseMotion(msg)

	case statusMsg:
		m.status.current = &msg
		return m, clearStatusAfter(statusDisplayDuration)

	case clearStatusMsg:
		m.status.current = nil
		return m, nil

	case showTooltipMsg:
		m.tooltip.show(msg.seq)
		return m, nil

	case snmp.ConnectMsg:
		if msg.Err != nil {
			return m.setStatusReturn(statusError, "Connect: "+msg.Err.Error())
		}
		m.snmp = msg.Session
		m.lastProfile = msg.Profile
		m.overlay.kind = overlayNone
		m.dialog = nil
		return m.setStatusReturn(statusSuccess, "Connected to "+msg.Session.Target)

	case snmp.DisconnectMsg:
		if m.walk != nil {
			m.walk.Cancel()
			m.walk = nil
			m.results.walkStatus = ""
		}
		m.snmp = nil
		return m.setStatusReturn(statusInfo, "Disconnected")

	case snmp.GetMsg:
		return m.handleGetResult(msg)

	case snmp.GetNextMsg:
		return m.handleGetNextResult(msg)

	case snmp.WalkBatchMsg:
		return m.handleWalkBatch(msg)

	case snmp.TableDataMsg:
		return m.handleTableData(msg)

	case deviceDialogSubmitMsg:
		m.overlay.kind = overlayNone
		m.dialog = nil
		return m, snmp.ConnectCmd(msg.profile)

	case snapshotMsg:
		if msg.err != nil {
			return m.setStatusReturn(statusError, "Snapshot failed: "+msg.err.Error())
		}
		return m.setStatusReturn(statusSuccess, "Snapshot: "+msg.path)

	case deviceDialogDeleteMsg:
		if m.profiles != nil {
			m.profiles.Remove(msg.name)
			if err := m.profiles.Save(); err != nil {
				return m.setStatusReturn(statusError, "Profile delete failed: "+err.Error())
			}
			return m.setStatusReturn(statusInfo, "Profile removed: "+msg.name)
		}
		return m, nil

	case tea.KeyPressMsg:
		// Any key press hides tooltip
		m.tooltip.hide()

		// Context menu keyboard handling
		if m.contextMenu.visible {
			return m.handleContextMenuKey(msg)
		}

		// Help overlay swallows all keys except dismiss
		if m.overlay.kind == overlayHelp {
			switch msg.String() {
			case "?", "esc", "q":
				m.overlay.kind = overlayNone
			}
			return m, nil
		}

		// Filter help overlay: dismiss and restore filter focus
		if m.overlay.kind == overlayFilterHelp {
			switch msg.String() {
			case "?", "esc", "q":
				m.overlay.kind = overlayNone
				m.focus = focusFilter
				return m, m.filterBar.input.Focus()
			}
			return m, nil
		}

		// Connection dialog swallows all keys
		if m.overlay.kind == overlayConnect && m.dialog != nil {
			cmd, closed := m.dialog.update(msg)
			if closed {
				m.overlay.kind = overlayNone
				m.dialog = nil
			}
			return m, cmd
		}

		// Pending chord: resolve or cancel
		if m.pendingChord != "" {
			prefix := m.pendingChord
			m.pendingChord = ""
			if msg.String() == "esc" {
				return m, nil
			}
			return m.resolveChord(prefix, msg.String())
		}

		// Chord prefix activation (only in tree/results/detail focus)
		if m.focus == focusTree || m.focus == focusResults || m.focus == focusDetail {
			switch msg.String() {
			case "s", "c", "v":
				m.pendingChord = msg.String()
				return m, nil
			}
		}

		// Global keys shared by tree, results, and detail
		if m.focus == focusTree || m.focus == focusResults || m.focus == focusDetail {
			if ret, retCmd, handled := m.handleGlobalKeys(msg); handled {
				return ret, retCmd
			}
		}

		// Focus-specific dispatch
		switch m.focus {
		case focusTree:
			return m.updateTree(msg)
		case focusSearch:
			return m.updateSearch(msg)
		case focusFilter:
			return m.updateFilter(msg)
		case focusDetail:
			return m.updateDetail(msg)
		case focusDiag:
			return m.updateDiag(msg)
		case focusModule:
			return m.updateModule(msg)
		case focusTypes:
			return m.updateTypes(msg)
		case focusQueryBar:
			return m.updateQueryBar(msg)
		case focusResults:
			return m.updateResults(msg)
		case focusResultFilter:
			return m.updateResultFilter(msg)
		case focusXref:
			return m.updateXref(msg)
		}
	}
	return m, nil
}

// syncSelection updates the detail/table pane after a tree cursor change.
func (m *model) syncSelection() {
	if node := m.tree.selectedNode(); node != nil {
		m.detail.setNode(node)
		if m.topPane == topTableSchema {
			if !m.tableSchema.setNode(node) {
				m.topPane = topDetail
			}
		}
	}
}

// syncResultSelection updates the detail pane after a result cursor change.
// Only updates when topPane is detail (avoids clobbering diagnostics/modules/types).
func (m *model) syncResultSelection() {
	if m.topPane != topDetail {
		return
	}
	var node *mib.Node
	if m.results.treeMode {
		node = m.results.selectedTreeNode()
	} else {
		res := m.results.selectedResult()
		if res != nil {
			oid, err := mib.ParseOID(res.OID)
			if err == nil {
				node = m.mib.LongestPrefixByOID(oid)
			}
		}
	}
	if node != nil {
		m.detail.setNode(node)
	}
}

// crossRefResult jumps the MIB tree to the given node.
func (m *model) crossRefResult(node *mib.Node) {
	m.navPush()
	m.tree.jumpToNode(node)
	m.detail.setNode(node)
	m.topPane = topDetail
	m.focus = focusTree
}

// crossRefResultByOID parses an OID string and jumps to the matching MIB node.
func (m *model) crossRefResultByOID(oidStr string) {
	oid, err := mib.ParseOID(oidStr)
	if err != nil {
		return
	}
	node := m.mib.LongestPrefixByOID(oid)
	if node == nil {
		return
	}
	m.crossRefResult(node)
}

func copyToClipboard(node *mib.Node) tea.Cmd {
	oid := node.OID()
	if oid == nil {
		return nil
	}
	return copyText(oid.String())
}

// openContextMenu builds and shows the context menu for the pane at (x, y).
func (m model) openContextMenu(x, y int) (tea.Model, tea.Cmd) {
	l := m.generateLayout()
	pt := image.Pt(x, y)

	var items []contextMenuItem

	if pt.In(l.tree) {
		// Select the tree row under the cursor before building menu
		row := y - l.tree.Min.Y + m.tree.lv.Offset()
		if row >= 0 && row < m.tree.lv.Len() {
			m.tree.lv.SetCursor(row)
			m.syncSelection()
		}
		items = treeMenuItems(m)
	} else if pt.In(l.rightBot) && l.rightBot.Dy() > 0 {
		switch m.bottomPane {
		case bottomResults:
			row := y - l.rightBot.Min.Y - resultHeaderLines + m.results.dataOffset()
			m.results.clickRow(row)
			m.syncResultSelection()
			items = resultMenuItems(m)
		case bottomTableData:
			row := y - l.rightBot.Min.Y - tableDataHeaderLines + m.tableData.offset
			if row >= 0 && row < len(m.tableData.rows) {
				m.tableData.cursor = row
				m.tableData.ensureVisible()
			}
			items = tableDataMenuItems(m)
		}
	} else if pt.In(l.rightTop) {
		items = detailMenuItems(m)
	}

	// Don't show empty menus or menus with only separators
	hasAction := false
	for _, item := range items {
		if !item.isSep {
			hasAction = true
			break
		}
	}
	if !hasAction {
		return m, nil
	}

	m.contextMenu.open(items, x, y)
	return m, nil
}

// contextMenuSelectAt handles a left-click at (x, y) inside the context menu.
func (m model) contextMenuSelectAt(x, y int) (tea.Model, tea.Cmd) {
	l := m.generateLayout()
	idx := m.contextMenu.itemAtY(y, l.area)
	if idx >= 0 && idx < len(m.contextMenu.items) {
		item := &m.contextMenu.items[idx]
		if item.enabled && !item.isSep && item.action != nil {
			m.contextMenu.close()
			return item.action(m)
		}
	}
	m.contextMenu.close()
	return m, nil
}
