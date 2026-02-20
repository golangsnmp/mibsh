package main

import (
	"context"
	"errors"
	"fmt"
	"image"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/ultraviolet/layout"
	"github.com/golangsnmp/gomib/mib"
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

// setStatusReturn sets a status message and returns the model with a clear-after command.
func (m model) setStatusReturn(typ statusType, text string) (tea.Model, tea.Cmd) {
	m.status.current = &statusMsg{typ: typ, text: text}
	return m, clearStatusAfter(statusDisplayDuration)
}

// requireConnectedIdle checks that the SNMP session is connected and no walk is
// in progress. Returns false with an appropriate status message if either check fails.
func (m model) requireConnectedIdle() (tea.Model, tea.Cmd, bool) {
	if m.snmp == nil || !m.snmp.connected {
		ret, cmd := m.setStatusReturn(statusError, "Not connected")
		return ret, cmd, false
	}
	if m.walk != nil {
		ret, cmd := m.setStatusReturn(statusWarn, "Walk in progress")
		return ret, cmd, false
	}
	return m, nil, true
}

// requireSelectedOID returns the selected tree node and its OID, validating that
// the OID is deep enough for SNMP operations. Returns ok=false if the node is nil,
// has no OID, or the OID is too short.
func (m model) requireSelectedOID() (*mib.Node, mib.OID, tea.Model, tea.Cmd, bool) {
	node := m.tree.selectedNode()
	if node == nil {
		return nil, nil, m, nil, false
	}
	oid := node.OID()
	if oid == nil {
		return nil, nil, m, nil, false
	}
	if len(oid) < 2 {
		ret, cmd := m.setStatusReturn(statusWarn, "OID too short for SNMP, select a node deeper in the tree")
		return nil, nil, ret, cmd, false
	}
	return node, oid, m, nil, true
}

// resolveChord dispatches a chord second-key press.
func (m model) resolveChord(prefix, key string) (tea.Model, tea.Cmd) {
	switch prefix + key {
	// SNMP operations
	case "sg":
		return m.snmpGet()
	case "sn":
		return m.snmpGetNext()
	case "sw":
		return m.snmpWalk()
	case "st":
		return m.snmpTableData()
	case "sq":
		m.focus = focusQueryBar
		m.queryBar.activate()
		return m, m.queryBar.focusCmd()

	// Connection
	case "cc":
		return m.openConnectDialog()
	case "cd":
		return m.snmpDisconnect()
	case "cs":
		return m.saveProfile()

	// View switching
	case "vd":
		m.topPane = topDiag
		m.focus = focusDiag
		m.diag.activate()
		m.updateLayout()
		return m, m.diag.input.Focus()
	case "vm":
		m.topPane = topModule
		m.focus = focusModule
		m.module.activate()
		m.updateLayout()
		return m, m.module.input.Focus()
	case "vy":
		m.topPane = topTypes
		m.focus = focusTypes
		m.typeBrowser.activate()
		m.updateLayout()
		return m, m.typeBrowser.input.Focus()
	case "vs":
		if m.topPane == topTableSchema {
			m.topPane = topDetail
		} else {
			if node := m.tree.selectedNode(); node != nil {
				if m.tableSchema.setNode(node) {
					m.topPane = topTableSchema
				}
			}
		}
		return m, nil
	case "vr":
		if !m.results.history.isEmpty() {
			m.bottomPane = bottomResults
			m.focus = focusResults
			m.updateLayout()
		}
		return m, nil
	case "vi":
		m.detail.devMode = !m.detail.devMode
		m.detail.viewport.SetContent(m.detail.buildContent())
		m.detail.viewport.GotoTop()
		return m, nil
	case "vt":
		m.results.toggleTreeMode()
		return m, nil
	case "vo":
		m.results.showRawOID = !m.results.showRawOID
		return m, nil

	// Tree pane resize (chord stays active for repeated taps)
	case "v,":
		m.treeWidthPct = max(15, m.treeWidthPct-5)
		m.pendingChord = "v"
		m.updateLayout()
		return m, nil
	case "v.":
		m.treeWidthPct = min(70, m.treeWidthPct+5)
		m.pendingChord = "v"
		m.updateLayout()
		return m, nil
	}

	// Unrecognized second key, swallow it
	return m, nil
}

// handleGlobalKeys handles keys shared across tree and results focus modes.
// Returns (model, cmd, true) if handled, (model, nil, false) if not.
func (m model) handleGlobalKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit, true
	case "?":
		m.overlay.kind = overlayHelp
		return m, nil, true
	case "esc":
		if m.walk != nil {
			m.walk.cancel()
			m.results.walkStatus = "cancelling..."
			return m, nil, true
		}
		if m.focus == focusResults {
			if m.results.isFiltering() {
				m.results.clearFilter()
				return m, nil, true
			}
			m.focus = focusTree
			return m, nil, true
		}
		if m.focus == focusDetail {
			m.focus = focusTree
			return m, nil, true
		}
		return m, nil, true
	case "/":
		if m.focus == focusResults && m.bottomPane != bottomNone {
			m.focus = focusResultFilter
			m.results.activateFilter()
			return m, m.results.filterInput.Focus(), true
		}
		m.focus = focusSearch
		m.search.activate()
		m.updateLayout()
		return m, m.search.input.Focus(), true
	case "f":
		m.focus = focusFilter
		m.filterBar.activate()
		m.updateLayout()
		return m, m.filterBar.input.Focus(), true
	case "~":
		ret, cmd := m.snapshot()
		return ret, cmd, true
	case "y":
		if node := m.tree.selectedNode(); node != nil {
			return m, copyToClipboard(node), true
		}
		return m, nil, true
	case "J":
		m.scrollTopPaneDown()
		return m, nil, true
	case "K":
		m.scrollTopPaneUp()
		return m, nil, true
	case "<":
		if m.bottomPane == bottomTableData {
			m.tableData.scrollLeft()
		}
		return m, nil, true
	case ">":
		if m.bottomPane == bottomTableData {
			m.tableData.scrollRight()
		}
		return m, nil, true
	case "[":
		if m.bottomPane == bottomResults {
			m.results.historyPrev()
			m.syncResultSelection()
		}
		return m, nil, true
	case "]":
		if m.bottomPane == bottomResults {
			m.results.historyNext()
			m.syncResultSelection()
		}
		return m, nil, true
	case "backspace":
		m.navPop()
		return m, nil, true
	case "x":
		if node := m.tree.selectedNode(); node != nil {
			if refs := m.detail.xrefs[node.Name()]; len(refs) > 0 {
				m.xrefPicker.activate(node.Name(), refs)
				m.focus = focusXref
				return m, nil, true
			}
		}
		return m, nil, true
	case "tab":
		switch m.focus {
		case focusResults:
			m.focus = focusTree
		case focusDetail:
			if m.bottomPane != bottomNone {
				m.focus = focusResults
				m.syncResultSelection()
			} else {
				m.focus = focusTree
			}
		default: // focusTree
			m.focus = focusDetail
		}
		return m, nil, true
	}
	return m, nil, false
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.SetWidth(msg.Width)
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

	case snmpConnectMsg:
		if msg.err != nil {
			return m.setStatusReturn(statusError, "Connect: "+msg.err.Error())
		}
		m.snmp = msg.session
		m.lastProfile = msg.profile
		m.overlay.kind = overlayNone
		m.dialog = nil
		return m.setStatusReturn(statusSuccess, "Connected to "+msg.session.target)

	case snmpDisconnectMsg:
		if m.walk != nil {
			m.walk.cancel()
			m.walk = nil
			m.results.walkStatus = ""
		}
		m.snmp = nil
		return m.setStatusReturn(statusInfo, "Disconnected")

	case snmpGetMsg:
		return m.handleGetResult(msg)

	case snmpGetNextMsg:
		return m.handleGetNextResult(msg)

	case snmpWalkBatchMsg:
		return m.handleWalkBatch(msg)

	case snmpTableDataMsg:
		return m.handleTableData(msg)

	case deviceDialogSubmitMsg:
		m.overlay.kind = overlayNone
		m.dialog = nil
		return m, connectCmd(msg.profile)

	case snapshotMsg:
		if msg.err != nil {
			return m.setStatusReturn(statusError, "Snapshot failed: "+msg.err.Error())
		}
		return m.setStatusReturn(statusSuccess, "Snapshot: "+msg.path)

	case deviceDialogDeleteMsg:
		if m.profiles != nil {
			m.profiles.remove(msg.name)
			if err := m.profiles.save(); err != nil {
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

func (m model) updateTree(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		m.tree.cursorDown()
	case "k", "up":
		m.tree.cursorUp()
	case "enter", "l", "right":
		m.tree.toggle()
	case "h", "left":
		m.tree.collapse()
	case "home":
		m.tree.goTop()
	case "G", "end":
		m.tree.goBottom()
	case "ctrl+d", "pgdown":
		m.tree.pageDown()
	case "ctrl+u", "pgup":
		m.tree.pageUp()
	}

	m.syncSelection()
	return m, nil
}

func (m model) updateDetail(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		m.scrollTopPaneDown()
	case "k", "up":
		m.scrollTopPaneUp()
	case "ctrl+d", "pgdown":
		m.scrollTopPaneBy(m.detail.height / 2)
	case "ctrl+u", "pgup":
		m.scrollTopPaneBy(-(m.detail.height / 2))
	case "home":
		m.detail.viewport.GotoTop()
	case "G", "end":
		m.detail.viewport.GotoBottom()
	}
	return m, nil
}

func (m model) updateSearch(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.search.deactivate()
		m.focus = focusTree
		m.updateLayout()
		return m, nil
	case "enter":
		if node := m.search.selectedNode(); node != nil {
			m.navPush()
			m.tree.jumpToNode(node)
			m.detail.setNode(node)
		}
		m.search.deactivate()
		m.focus = focusTree
		m.updateLayout()
		return m, nil
	case "up", "ctrl+p":
		m.search.prevResult()
		return m, nil
	case "down", "ctrl+n":
		m.search.nextResult()
		return m, nil
	}

	// Forward to text input
	var cmd tea.Cmd
	m.search.input, cmd = m.search.input.Update(msg)
	m.search.filter()
	return m, cmd
}

func (m model) updateFilter(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if m.filterBar.input.Value() != "" {
			// Clear filter, stay in filter bar
			m.filterBar.clear()
			m.tree.filterActive = false
			m.tree.rebuild()
			return m, nil
		}
		// Empty input, deactivate and return to tree
		m.filterBar.deactivate()
		m.focus = focusTree
		m.updateLayout()
		return m, nil
	case "enter":
		// Accept filter (stays active), return to tree
		m.filterBar.deactivate()
		m.focus = focusTree
		m.updateLayout()
		return m, nil
	case "?":
		m.overlay.kind = overlayFilterHelp
		return m, nil
	case "tab":
		m.filterBar.tabComplete()
		return m, nil
	}

	// Forward to text input
	var cmd tea.Cmd
	m.filterBar.input, cmd = m.filterBar.input.Update(msg)
	m.filterBar.resetCompletion()
	m.filterBar.recompile()
	m.tree.filterActive = m.filterBar.isFiltering()
	m.tree.rebuild()
	return m, cmd
}

// navigablePane is implemented by sub-models that support cursor navigation
// (diag, module). Used by handlePaneNav to share key handling.
type navigablePane interface {
	cursorUp()
	cursorDown()
	goTop()
	goBottom()
	pageUp()
	pageDown()
	deactivate()
}

// handlePaneNav handles shared navigation keys for top-right sub-panes.
// Returns true if the key was handled.
func (m model) handlePaneNav(pane navigablePane, msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.String() {
	case "esc":
		pane.deactivate()
		m.topPane = topDetail
		m.focus = focusTree
		m.updateLayout()
		return m, nil, true
	case "up", "ctrl+p":
		pane.cursorUp()
		return m, nil, true
	case "down", "ctrl+n":
		pane.cursorDown()
		return m, nil, true
	case "home":
		pane.goTop()
		return m, nil, true
	case "end":
		pane.goBottom()
		return m, nil, true
	case "pgup", "ctrl+u":
		pane.pageUp()
		return m, nil, true
	case "pgdown", "ctrl+d":
		pane.pageDown()
		return m, nil, true
	}
	return m, nil, false
}

func (m model) updateDiag(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if ret, retCmd, handled := m.handlePaneNav(&m.diag, msg); handled {
		return ret, retCmd
	}
	switch msg.String() {
	case "tab":
		m.diag.cycleSeverity()
		return m, nil
	case "enter":
		if diag := m.diag.selectedDiag(); diag != nil && diag.Module != "" {
			if node := m.findModuleNode(diag.Module); node != nil {
				m.navPush()
				m.diag.deactivate()
				m.topPane = topDetail
				m.focus = focusTree
				m.tree.jumpToNode(node)
				m.detail.setNode(node)
				m.updateLayout()
			}
		}
		return m, nil
	}

	// Forward to text input
	var cmd tea.Cmd
	m.diag.input, cmd = m.diag.input.Update(msg)
	m.diag.applyFilter()
	return m, cmd
}

func (m model) updateModule(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if ret, retCmd, handled := m.handlePaneNav(&m.module, msg); handled {
		return ret, retCmd
	}
	switch msg.String() {
	case "enter":
		m.module.toggleExpand()
		return m, nil
	}

	// Forward to text input
	var cmd tea.Cmd
	m.module.input, cmd = m.module.input.Update(msg)
	m.module.applyFilter()
	return m, cmd
}

func (m model) updateTypes(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if ret, retCmd, handled := m.handlePaneNav(&m.typeBrowser, msg); handled {
		return ret, retCmd
	}
	switch msg.String() {
	case "enter":
		m.typeBrowser.toggleExpand()
		return m, nil
	case "tab":
		m.typeBrowser.cycleTCFilter()
		return m, nil
	}

	// Forward to text input
	var cmd tea.Cmd
	m.typeBrowser.input, cmd = m.typeBrowser.input.Update(msg)
	m.typeBrowser.applyFilter()
	return m, cmd
}

func (m model) updateQueryBar(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.queryBar.blur()
		m.focus = focusTree
		return m, nil
	case "enter":
		cmd := m.queryBar.parse()
		if cmd == nil {
			// Parse error or empty - stay in query bar (error shown in view)
			if m.queryBar.input.Value() == "" {
				// Empty input, just close
				m.queryBar.blur()
				m.focus = focusTree
			}
			return m, nil
		}
		m.queryBar.blur()
		m.focus = focusTree
		return m.dispatchQuery(*cmd)
	case "tab":
		m.queryBar.tabComplete()
		return m, nil
	}

	// Forward to text input
	var teaCmd tea.Cmd
	m.queryBar.input, teaCmd = m.queryBar.input.Update(msg)
	m.queryBar.resetCompletion()
	m.queryBar.err = ""
	return m, teaCmd
}

func (m model) updateResults(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		m.results.cursorDown()
	case "k", "up":
		m.results.cursorUp()
	case "ctrl+d", "pgdown":
		m.results.pageDown()
	case "ctrl+u", "pgup":
		m.results.pageUp()
	case "home":
		m.results.goTop()
	case "G", "end":
		m.results.goBottom()
	case "enter":
		// Enter does cross-reference (jumps tree + sets detail + changes focus),
		// so it keeps its own early return.
		if m.results.treeMode {
			row := m.results.selectedTreeNode()
			if row != nil {
				m.crossRefResult(row)
			} else {
				m.results.toggleTreeNode()
			}
		} else {
			res := m.results.selectedResult()
			if res != nil {
				m.crossRefResultByOID(res.oid)
			}
		}
		return m, nil
	case "h", "left":
		if m.results.treeMode {
			m.results.collapseTreeNode()
		}
	case "l", "right":
		if m.results.treeMode {
			m.results.expandTreeNode()
		}
	default:
		return m, nil
	}
	m.syncResultSelection()
	return m, nil
}

func (m model) updateResultFilter(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if m.results.filterInput.Value() != "" {
			// Clear filter, stay in result filter mode
			m.results.clearFilter()
			m.results.filterInput.SetValue("")
			return m, nil
		}
		// Empty input, close filter and return to results
		m.results.deactivateFilter()
		m.focus = focusResults
		return m, nil
	case "enter":
		// Accept filter, return to results focus
		m.results.deactivateFilter()
		m.focus = focusResults
		return m, nil
	}

	// Forward to text input
	var cmd tea.Cmd
	m.results.filterInput, cmd = m.results.filterInput.Update(msg)
	m.results.applyFilter()
	return m, cmd
}

func (m model) updateXref(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if ret, retCmd, handled := m.handlePaneNav(&m.xrefPicker, msg); handled {
		return ret, retCmd
	}
	switch msg.String() {
	case "enter":
		if sel := m.xrefPicker.selectedXref(); sel != nil {
			if node := m.mib.Node(sel.name); node != nil {
				m.navPush()
				m.xrefPicker.deactivate()
				m.topPane = topDetail
				m.focus = focusTree
				m.tree.jumpToNode(node)
				m.syncSelection()
			}
		}
		return m, nil
	}
	return m, nil
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

// dispatchQuery executes a parsed query bar command.
func (m model) dispatchQuery(cmd queryCmd) (tea.Model, tea.Cmd) {
	if ret, retCmd, ok := m.requireConnectedIdle(); !ok {
		return ret, retCmd
	}

	if parsed, err := mib.ParseOID(cmd.oid); err == nil && len(parsed) < 2 {
		return m.setStatusReturn(statusWarn, "OID too short for SNMP, use at least 2 arcs (e.g. 1.3)")
	}

	switch cmd.op {
	case queryGet:
		return m, getCmd(m.snmp, []string{cmd.oid})
	case queryGetNext:
		return m, getNextCmd(m.snmp, cmd.oid)
	case queryWalk:
		return m.startQueryWalk(cmd.oid)
	}
	return m, nil
}

// startQueryWalk starts a walk from a query bar OID (not from tree selection).
func (m model) startQueryWalk(oidStr string) (tea.Model, tea.Cmd) {
	walkOID, _ := mib.ParseOID(oidStr)

	// Try to find a name for the label
	label := "WALK " + oidStr
	if walkOID != nil {
		if node := m.mib.NodeByOID(walkOID); node != nil {
			label = "WALK " + node.Name()
		} else if node := m.mib.LongestPrefixByOID(walkOID); node != nil {
			label = "WALK " + node.Name()
		}
	}

	ws, cmd := startWalkCmd(m.snmp, oidStr)
	m.walk = ws

	g := resultGroup{
		op:          opWalk,
		label:       label,
		walkRootOID: walkOID,
	}
	m.results.addGroup(g)
	m.results.walkStatus = "walking..."
	m.bottomPane = bottomResults
	m.focus = focusResults
	m.updateLayout()

	s := statusMsg{typ: statusInfo, text: label + "..."}
	m.status.current = &s
	return m, cmd
}

func (m model) handleMouseMotion(msg tea.MouseMotionMsg) (tea.Model, tea.Cmd) {
	// Track mouse over context menu items
	if m.contextMenu.visible {
		l := m.generateLayout()
		if m.contextMenu.containsPoint(msg.X, msg.Y, l.area) {
			idx := m.contextMenu.itemAtY(msg.Y, l.area)
			if idx >= 0 && idx < len(m.contextMenu.items) && !m.contextMenu.items[idx].isSep && m.contextMenu.items[idx].enabled {
				m.contextMenu.cursor = idx
			}
		}
		return m, nil // suppress tooltip while menu is open
	}

	l := m.generateLayout()
	pt := image.Pt(msg.X, msg.Y)

	if pt.In(l.tree) {
		row := msg.Y - l.tree.Min.Y + m.tree.lv.Offset()
		if row >= 0 && row < m.tree.lv.Len() {
			r := m.tree.lv.Row(row)
			iconX := l.tree.Min.X + 1 + 2 + r.depth*2
			onIcon := r.hasKids && msg.X >= iconX && msg.X < iconX+2
			if onIcon {
				m.hoverRow = -1
				m.tooltip.hide()
			} else if row != m.hoverRow {
				m.hoverRow = row
				node := r.node
				return m, m.tooltip.startDelay(node, msg.X, msg.Y)
			}
		} else {
			m.hoverRow = -1
			m.tooltip.hide()
		}
	} else {
		if m.hoverRow != -1 || m.tooltip.visible {
			m.hoverRow = -1
			m.tooltip.hide()
		}
	}

	return m, nil
}

func (m model) handleMouseClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	// Dismiss open context menu on any click
	if m.contextMenu.visible {
		l := m.generateLayout()
		if m.contextMenu.containsPoint(msg.X, msg.Y, l.area) && msg.Button == tea.MouseLeft {
			return m.contextMenuSelectAt(msg.X, msg.Y)
		}
		m.contextMenu.close()
		// Fall through: right-click opens new menu, left-click does normal handling
	}

	if msg.Button == tea.MouseRight {
		m.tooltip.hide()
		m.pendingChord = ""
		return m.openContextMenu(msg.X, msg.Y)
	}

	if msg.Button != tea.MouseLeft {
		return m, nil
	}

	l := m.generateLayout()
	pt := image.Pt(msg.X, msg.Y)

	if pt.In(l.tree) {
		row := msg.Y - l.tree.Min.Y + m.tree.lv.Offset()
		if row >= 0 && row < m.tree.lv.Len() {
			now := time.Now()
			isDouble := row == m.lastClickRow &&
				now.Sub(m.lastClickAt) < doubleClickThreshold
			m.lastClickRow = row
			m.lastClickAt = now

			m.tree.lv.SetCursor(row)

			// Toggle on double-click or single-click on the icon
			r := m.tree.lv.Row(row)
			iconX := l.tree.Min.X + 1 + 2 + r.depth*2 // +1 pane padding, +2 for border/space
			clickedIcon := r.hasKids && msg.X >= iconX && msg.X < iconX+2
			if isDouble || clickedIcon {
				m.tree.toggle()
			}
			m.syncSelection()
		}
		// Click on tree returns to tree focus
		switch m.focus {
		case focusSearch:
			m.search.deactivate()
			m.focus = focusTree
			m.updateLayout()
		case focusDiag:
			m.diag.deactivate()
			m.topPane = topDetail
			m.focus = focusTree
			m.updateLayout()
		case focusModule:
			m.module.deactivate()
			m.topPane = topDetail
			m.focus = focusTree
			m.updateLayout()
		case focusTypes:
			m.typeBrowser.deactivate()
			m.topPane = topDetail
			m.focus = focusTree
			m.updateLayout()
		case focusFilter:
			m.filterBar.deactivate()
			m.focus = focusTree
			m.updateLayout()
		case focusQueryBar:
			m.queryBar.blur()
			m.focus = focusTree
		case focusResults:
			m.focus = focusTree
		case focusDetail:
			m.focus = focusTree
		}
	} else if pt.In(l.rightTop) {
		m.focus = focusDetail
	} else if pt.In(l.rightBot) {
		m.focus = focusResults
		switch m.bottomPane {
		case bottomResults:
			row := msg.Y - l.rightBot.Min.Y - resultHeaderLines + m.results.dataOffset()
			m.results.clickRow(row)

			// Double-click or icon-click to toggle in tree mode
			if m.results.treeMode && row >= 0 && row < m.results.treeLV.Len() {
				now := time.Now()
				isDouble := row == m.lastResultClickRow &&
					now.Sub(m.lastResultClickAt) < doubleClickThreshold
				m.lastResultClickRow = row
				m.lastResultClickAt = now

				r := m.results.treeLV.Row(row)
				// 2 chars for selection border area, then depth*2 indent
				iconX := l.rightBot.Min.X + 1 + 2 + r.depth*2
				clickedIcon := r.hasKids && msg.X >= iconX && msg.X < iconX+2
				if isDouble || clickedIcon {
					m.results.toggleTreeNode()
				}
			}
			m.syncResultSelection()
		case bottomTableData:
			row := msg.Y - l.rightBot.Min.Y - tableDataHeaderLines + m.tableData.offset
			if row >= 0 && row < len(m.tableData.rows) {
				m.tableData.cursor = row
				m.tableData.ensureVisible()
			}
		}
	}

	return m, nil
}

func (m model) handleMouseWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	if m.contextMenu.visible {
		m.contextMenu.close()
	}

	l := m.generateLayout()
	pt := image.Pt(msg.X, msg.Y)

	switch msg.Button {
	case tea.MouseWheelUp:
		if pt.In(l.tree) {
			m.tree.cursorBy(-mouseScrollLines)
			m.syncSelection()
		} else if pt.In(l.rightTop) {
			m.scrollTopPaneBy(-mouseScrollLines)
		} else if pt.In(l.rightBot) {
			m.scrollBottomPaneBy(-mouseScrollLines)
		}

	case tea.MouseWheelDown:
		if pt.In(l.tree) {
			m.tree.cursorBy(mouseScrollLines)
			m.syncSelection()
		} else if pt.In(l.rightTop) {
			m.scrollTopPaneBy(mouseScrollLines)
		} else if pt.In(l.rightBot) {
			m.scrollBottomPaneBy(mouseScrollLines)
		}
	}

	return m, nil
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
			oid, err := mib.ParseOID(res.oid)
			if err == nil {
				node = m.mib.LongestPrefixByOID(oid)
			}
		}
	}
	if node != nil {
		m.detail.setNode(node)
	}
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

func (m *model) scrollTopPaneUp() {
	switch m.topPane {
	case topDiag:
		m.diag.cursorUp()
	case topModule:
		m.module.cursorUp()
	case topTypes:
		m.typeBrowser.cursorUp()
	case topTableSchema:
		m.tableSchema.scrollUp()
	default:
		m.detail.scrollUp()
	}
}

func (m *model) scrollTopPaneDown() {
	switch m.topPane {
	case topDiag:
		m.diag.cursorDown()
	case topModule:
		m.module.cursorDown()
	case topTypes:
		m.typeBrowser.cursorDown()
	case topTableSchema:
		m.tableSchema.scrollDown()
	default:
		m.detail.scrollDown()
	}
}

// scrollTopPaneBy scrolls the top-right pane by n lines (positive = down, negative = up).
func (m *model) scrollTopPaneBy(n int) {
	switch m.topPane {
	case topDiag:
		m.diag.cursorBy(n)
	case topModule:
		m.module.cursorBy(n)
	case topTypes:
		m.typeBrowser.cursorBy(n)
	case topTableSchema:
		if n > 0 {
			m.tableSchema.scrollDownBy(n)
		} else {
			m.tableSchema.scrollUpBy(-n)
		}
	default:
		if n > 0 {
			m.detail.scrollDownBy(n)
		} else {
			m.detail.scrollUpBy(-n)
		}
	}
}

// scrollBottomPaneBy scrolls the bottom-right pane by n lines.
func (m *model) scrollBottomPaneBy(n int) {
	switch m.bottomPane {
	case bottomResults:
		if n > 0 {
			m.results.pageDown()
		} else {
			m.results.pageUp()
		}
		m.syncResultSelection()
	case bottomTableData:
		if n > 0 {
			m.tableData.pageDown()
		} else {
			m.tableData.pageUp()
		}
	}
}

func (m *model) generateLayout() appLayout {
	area := image.Rect(0, 0, m.width, m.height)

	// 1-cell horizontal margins (left/right border columns drawn here)
	area.Min.X += 1
	area.Max.X -= 1

	// Header bar (1 row)
	headerArea, rest := layout.SplitVertical(area, layout.Fixed(1))

	// Bottom: hint bar (2 rows) / filter / query / search
	bottomH := 2
	if m.focus == focusSearch {
		bottomH += min(maxSearchVisible+1, (m.height-2)/3)
	}

	mainArea, bottomArea := layout.SplitVertical(rest, layout.Fixed(rest.Dy()-bottomH))

	// Reserve 1 row each for top and bottom border lines
	contentArea := mainArea
	contentArea.Min.Y += 1
	contentArea.Max.Y -= 1

	// Left/right split: tree | border col (1 col) | right pane
	treeAndSep, rightFull := layout.SplitHorizontal(contentArea, layout.Percent(m.treeWidthPct))
	treeRect, sepRect := layout.SplitHorizontal(treeAndSep, layout.Fixed(treeAndSep.Dx()-1))

	// Split right pane into top + separator + bottom when bottom pane is active
	var rightTop, rightSepRect, rightBot image.Rectangle
	if m.bottomPane == bottomNone {
		rightTop = rightFull
		// rightSepRect and rightBot stay zero
	} else {
		minH := 5
		if rightFull.Dy() < minH*2+1 {
			// Too short, give all space to top
			rightTop = rightFull
		} else {
			topPart, remainder := layout.SplitVertical(rightFull, layout.Percent(50))
			rightSepRect, rightBot = layout.SplitVertical(remainder, layout.Fixed(1))
			rightTop = topPart
		}
	}

	return appLayout{
		area:     image.Rect(0, 0, m.width, m.height),
		header:   headerArea,
		tree:     treeRect,
		sep:      sepRect,
		rightTop: rightTop,
		rightSep: rightSepRect,
		rightBot: rightBot,
		bottom:   bottomArea,
	}
}

func (m *model) applyLayout(l appLayout) {
	// Pane styles use Padding(0, 1) = 1 left + 1 right = 2 horizontal cells.
	// Sub-models receive the content-area width (total minus padding).
	const panePad = 2
	m.tree.setSize(max(0, l.tree.Dx()-panePad), l.tree.Dy())

	// Top-right sub-pane components
	m.detail.setSize(max(0, l.rightTop.Dx()-panePad), l.rightTop.Dy())
	m.tableSchema.setSize(max(0, l.rightTop.Dx()-panePad), l.rightTop.Dy())
	m.diag.setSize(max(0, l.rightTop.Dx()-panePad), l.rightTop.Dy())
	m.module.setSize(max(0, l.rightTop.Dx()-panePad), l.rightTop.Dy())
	m.typeBrowser.setSize(max(0, l.rightTop.Dx()-panePad), l.rightTop.Dy())
	m.xrefPicker.setSize(max(0, l.rightTop.Dx()-panePad), l.rightTop.Dy())

	// Bottom-right sub-pane components
	botRect := l.rightBot
	if botRect.Dy() == 0 {
		// When no bottom pane, size using the full right area (for results still showing)
		botRect = l.rightTop
	}
	m.results.setSize(max(0, botRect.Dx()-panePad), botRect.Dy())
	m.tableData.setSize(max(0, botRect.Dx()-panePad), botRect.Dy())
	m.search.setSize(m.width)
	m.filterBar.width = m.width
}

func (m *model) updateLayout() {
	l := m.generateLayout()
	m.applyLayout(l)
}

// findModuleNode returns the first node belonging to the named module.
func (m *model) findModuleNode(moduleName string) *mib.Node {
	for node := range m.mib.Nodes() {
		if node.Module() != nil && node.Module().Name() == moduleName {
			return node
		}
	}
	return nil
}

func (m model) handleDialogClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if msg.Button != tea.MouseLeft || m.dialog == nil {
		return m, nil
	}

	// Reconstruct the dialog box rect (same logic as drawCentered)
	l := m.generateLayout()
	content := m.dialog.view()
	box := styles.Dialog.Box.Render(content)
	w := lipgloss.Width(box)
	h := lipgloss.Height(box)
	rect := layout.CenterRect(l.area, w, h)

	// Dialog.Box has RoundedBorder (1 row) + Padding(1, 2) = 2 rows top offset
	contentY := msg.Y - rect.Min.Y - 2
	if contentY < 0 {
		return m, nil
	}

	// Check for profile click
	if pi := m.dialog.lineToProfile(contentY); pi >= 0 {
		m.dialog.profileIdx = pi
		m.dialog.section = sectionProfiles
		m.dialog.blurAll()
		return m, nil
	}

	// Check for field click
	f := m.dialog.lineToField(contentY)
	if f >= 0 {
		return m, m.dialog.focusFieldAt(f)
	}
	return m, nil
}

// snmpGet issues an SNMP GET for the currently selected tree node.
func (m model) snmpGet() (tea.Model, tea.Cmd) {
	if ret, retCmd, ok := m.requireConnectedIdle(); !ok {
		return ret, retCmd
	}

	node, oid, ret, retCmd, ok := m.requireSelectedOID()
	if !ok {
		return ret, retCmd
	}

	oidStr := oid.String()
	// Append .0 for scalar nodes
	if node.Kind() == mib.KindScalar {
		oidStr += ".0"
	}

	return m, getCmd(m.snmp, []string{oidStr})
}

// snmpGetNext issues an SNMP GETNEXT for the currently selected tree node.
func (m model) snmpGetNext() (tea.Model, tea.Cmd) {
	if ret, retCmd, ok := m.requireConnectedIdle(); !ok {
		return ret, retCmd
	}

	_, oid, ret, retCmd, ok := m.requireSelectedOID()
	if !ok {
		return ret, retCmd
	}

	return m, getNextCmd(m.snmp, oid.String())
}

func (m model) openConnectDialog() (tea.Model, tea.Cmd) {
	var profiles []deviceProfile
	if m.profiles != nil {
		profiles = m.profiles.profiles
	}
	d := newDeviceDialog(m.config, profiles)
	m.dialog = &d
	m.overlay.kind = overlayConnect
	return m, d.focusCmd()
}

func (m model) saveProfile() (tea.Model, tea.Cmd) {
	if m.profiles == nil || m.snmp == nil || !m.snmp.connected {
		return m.setStatusReturn(statusError, "Not connected")
	}

	p := m.lastProfile
	name := p.target
	dp := deviceProfile{
		Name:          name,
		Target:        p.target,
		Community:     p.community,
		Version:       p.version,
		SecurityLevel: p.securityLevel,
		Username:      p.username,
		AuthProto:     p.authProto,
		AuthPass:      p.authPass,
		PrivProto:     p.privProto,
		PrivPass:      p.privPass,
	}
	m.profiles.upsert(dp)
	if err := m.profiles.save(); err != nil {
		return m.setStatusReturn(statusError, "Save failed: "+err.Error())
	}

	return m.setStatusReturn(statusSuccess, "Profile saved: "+name)
}

func (m model) snmpDisconnect() (tea.Model, tea.Cmd) {
	if m.snmp == nil {
		return m, nil
	}
	return m, disconnectCmd(m.snmp)
}

func (m model) handleGetResult(msg snmpGetMsg) (tea.Model, tea.Cmd) {
	g := resultGroup{
		op:    opGet,
		label: "GET",
		err:   msg.err,
	}

	if msg.err == nil {
		for _, pdu := range msg.results {
			g.results = append(g.results, formatPDUToResult(pdu, m.mib))
		}
		g.label = "GET " + g.results[0].name
	}

	m.results.addGroup(g)
	m.bottomPane = bottomResults
	m.focus = focusResults
	m.updateLayout()

	if msg.err != nil {
		return m.setStatusReturn(statusError, "GET failed: "+msg.err.Error())
	}

	return m.setStatusReturn(statusSuccess, "GET "+g.results[0].name+": ok")
}

func (m model) handleGetNextResult(msg snmpGetNextMsg) (tea.Model, tea.Cmd) {
	g := resultGroup{
		op:    opGetNext,
		label: "GETNEXT " + msg.oid,
		err:   msg.err,
	}

	if msg.err == nil {
		for _, pdu := range msg.results {
			g.results = append(g.results, formatPDUToResult(pdu, m.mib))
		}
	}

	m.results.addGroup(g)
	m.bottomPane = bottomResults
	m.focus = focusResults
	m.updateLayout()

	if msg.err != nil {
		return m.setStatusReturn(statusError, "GETNEXT failed: "+msg.err.Error())
	}
	return m, nil
}

// snmpWalk starts a walk from the currently selected tree node's OID.
func (m model) snmpWalk() (tea.Model, tea.Cmd) {
	if ret, retCmd, ok := m.requireConnectedIdle(); !ok {
		return ret, retCmd
	}

	node, oid, ret, retCmd, ok := m.requireSelectedOID()
	if !ok {
		return ret, retCmd
	}

	oidStr := oid.String()
	label := "WALK " + node.Name()

	ws, cmd := startWalkCmd(m.snmp, oidStr)
	m.walk = ws

	g := resultGroup{
		op:          opWalk,
		label:       label,
		walkRootOID: oid,
	}
	m.results.addGroup(g)
	m.results.walkStatus = "walking..."
	m.bottomPane = bottomResults
	m.focus = focusResults
	m.updateLayout()

	s := statusMsg{typ: statusInfo, text: "Walking " + node.Name() + "..."}
	m.status.current = &s
	return m, cmd
}

func (m model) handleWalkBatch(msg snmpWalkBatchMsg) (tea.Model, tea.Cmd) {
	// Format and append results
	if len(msg.pdus) > 0 {
		results := make([]snmpResult, 0, len(msg.pdus))
		for _, pdu := range msg.pdus {
			results = append(results, formatPDUToResult(pdu, m.mib))
		}
		m.results.appendResults(results)
	}

	if !msg.done {
		if m.walk != nil {
			return m, waitWalkCmd(m.walk.ch)
		}
		// Walk was cancelled via disconnect, stop processing
		return m, nil
	}

	// Walk complete
	m.results.walkStatus = ""

	if m.walk == nil {
		// Already cleaned up by disconnect
		return m, nil
	}
	m.walk = nil

	g := m.results.history.current()
	count := 0
	if g != nil {
		count = len(g.results)
	}

	if msg.err != nil {
		if errors.Is(msg.err, context.Canceled) {
			s := statusMsg{typ: statusInfo, text: fmt.Sprintf("Walk cancelled (%d results)", count)}
			m.status.current = &s
		} else {
			if g != nil {
				g.err = msg.err
			}
			s := statusMsg{typ: statusError, text: "Walk failed: " + msg.err.Error()}
			m.status.current = &s
		}
	} else {
		s := statusMsg{typ: statusSuccess, text: fmt.Sprintf("Walk complete: %d results", count)}
		m.status.current = &s
	}

	return m, clearStatusAfter(statusDisplayDuration)
}

// snmpTableData fetches live table data for the currently selected table/row/column node.
func (m model) snmpTableData() (tea.Model, tea.Cmd) {
	if ret, retCmd, ok := m.requireConnectedIdle(); !ok {
		return ret, retCmd
	}

	node := m.tree.selectedNode()
	if node == nil {
		return m, nil
	}

	obj := node.Object()
	if obj == nil {
		return m, nil
	}

	tbl, _ := resolveTable(obj, node.Kind())

	if tbl == nil {
		return m.setStatusReturn(statusWarn, "Not a table node")
	}

	label := "TABLE " + tbl.Name()
	m.tableData.setLoading(label)
	m.bottomPane = bottomTableData
	m.focus = focusResults
	m.updateLayout()

	s := statusMsg{typ: statusInfo, text: "Fetching " + tbl.Name() + "..."}
	m.status.current = &s

	return m, tableWalkCmd(m.snmp, tbl, m.mib)
}

func (m model) handleTableData(msg snmpTableDataMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.tableData.setError(msg.err)
		return m.setStatusReturn(statusError, "Table fetch failed: "+msg.err.Error())
	}

	m.tableData.setData(msg.tableName, msg.columns, msg.rows, msg.indexCols)
	m.bottomPane = bottomTableData
	m.updateLayout()

	return m.setStatusReturn(statusSuccess, fmt.Sprintf("TABLE %s: %d rows", msg.tableName, len(msg.rows)))
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

// handleContextMenuKey handles keyboard input while the context menu is open.
func (m model) handleContextMenuKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.contextMenu.close()
		return m, nil
	case "j", "down":
		m.contextMenu.cursorDown()
		return m, nil
	case "k", "up":
		m.contextMenu.cursorUp()
		return m, nil
	case "enter":
		if item := m.contextMenu.selectedItem(); item != nil && item.action != nil {
			m.contextMenu.close()
			return item.action(m)
		}
		m.contextMenu.close()
		return m, nil
	default:
		// Any other key dismisses the menu
		m.contextMenu.close()
		return m, nil
	}
}
