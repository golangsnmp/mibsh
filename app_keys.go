package main

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/golangsnmp/gomib/mib"
	"github.com/golangsnmp/mibsh/internal/snmp"
)

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
	case "sp":
		return m.snmpWatch()
	case "sq":
		m.focus = focusQueryBar
		return m, m.queryBar.activate()

	// Connection
	case "cc":
		return m.openConnectDialog()
	case "cd":
		return m.snmpDisconnect()
	case "cs":
		return m.saveProfile()

	// View switching
	case "vd":
		m.diag.activate()
		return m.switchTopPane(topDiag, focusDiag, m.diag.input.Focus())
	case "vm":
		m.module.activate()
		return m.switchTopPane(topModule, focusModule, m.module.input.Focus())
	case "vy":
		m.typeBrowser.activate()
		return m.switchTopPane(topTypes, focusTypes, m.typeBrowser.input.Focus())
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
		if !m.results.history.IsEmpty() {
			m.bottomPane = bottomResults
			m.focus = focusResults
			m.updateLayout()
		}
		return m, nil
	case "vi":
		m.detail.devMode = !m.detail.devMode
		m.detail.vp.SetContent(m.detail.buildContent(m.xrefs))
		m.detail.vp.GotoTop()
		return m, nil
	case "vt":
		m.results.toggleTreeMode()
		return m, nil
	case "vo":
		m.results.showRawOID = !m.results.showRawOID
		return m, nil
	case "vc":
		return m.openColumnPicker()

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
		if m.watch.active {
			m.watch.stop()
			m.focus = focusTree
			return m, clearStatusAfter(0), true
		}
		if m.walk != nil {
			m.walk.Cancel()
			m.results.walkStatus = "cancelling..."
			return m, nil, true
		}
		if m.focus == focusWatch {
			m.focus = focusTree
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
		m.scrollTopPaneBy(1)
		return m, nil, true
	case "K":
		m.scrollTopPaneBy(-1)
		return m, nil, true
	case "<":
		switch m.bottomPane {
		case bottomTableData:
			m.tableData.scrollLeft()
		case bottomWatch:
			m.watch.scrollLeft()
		}
		return m, nil, true
	case ">":
		switch m.bottomPane {
		case bottomTableData:
			m.tableData.scrollRight()
		case bottomWatch:
			m.watch.scrollRight()
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
	case "+", "=":
		if m.watch.active {
			m.watch.adjustInterval(watchIntervalStep)
			m.setStatus(statusInfo, fmt.Sprintf("Watch interval: %s", formatInterval(m.watch.interval)))
			return m, clearStatusAfter(statusDisplayDuration), true
		}
		return m, nil, true
	case "-":
		if m.watch.active {
			m.watch.adjustInterval(-watchIntervalStep)
			m.setStatus(statusInfo, fmt.Sprintf("Watch interval: %s", formatInterval(m.watch.interval)))
			return m, clearStatusAfter(statusDisplayDuration), true
		}
		return m, nil, true
	case "backspace":
		m.navPop()
		return m, nil, true
	case "x":
		if node := m.tree.selectedNode(); node != nil {
			if refs := m.xrefs[node.Name()]; len(refs) > 0 {
				m.xrefPicker.activate(node.Name(), refs)
				m.focus = focusXref
				return m, nil, true
			}
		}
		return m, nil, true
	case "tab":
		switch m.focus {
		case focusResults, focusWatch:
			m.focus = focusTree
		case focusDetail:
			if m.bottomPane == bottomWatch {
				m.focus = focusWatch
			} else if m.bottomPane != bottomNone {
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
		m.scrollTopPaneBy(1)
	case "k", "up":
		m.scrollTopPaneBy(-1)
	case "ctrl+d", "pgdown":
		m.scrollTopPaneBy(m.detail.height / 2)
	case "ctrl+u", "pgup":
		m.scrollTopPaneBy(-(m.detail.height / 2))
	case "home":
		m.detail.vp.GotoTop()
	case "G", "end":
		m.detail.vp.GotoBottom()
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
			m.detail.setNode(node, m.xrefs)
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

// switchTopPane sets the top pane and focus, updates layout, and returns the
// given focus command. Used by resolveChord for vd/vm/vy view switching.
func (m model) switchTopPane(tp topPane, f focus, focusCmd tea.Cmd) (tea.Model, tea.Cmd) {
	m.topPane = tp
	m.focus = f
	m.updateLayout()
	return m, focusCmd
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
// It mutates only through the pane interface. Returns true if the key was handled.
// Callers must handle "esc" themselves before calling this function.
// Note: j/k are not included here because some callers (diag, module, types)
// forward unhandled keys to text inputs. Callers without text inputs should
// handle j/k separately.
func handlePaneNav(pane navigablePane, msg tea.KeyPressMsg) bool {
	switch msg.String() {
	case "down", "ctrl+n":
		pane.cursorDown()
		return true
	case "up", "ctrl+p":
		pane.cursorUp()
		return true
	case "home":
		pane.goTop()
		return true
	case "end":
		pane.goBottom()
		return true
	case "pgup", "ctrl+u":
		pane.pageUp()
		return true
	case "pgdown", "ctrl+d":
		pane.pageDown()
		return true
	}
	return false
}

func (m model) updateDiag(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.diag.deactivate()
		m.topPane = topDetail
		m.focus = focusTree
		m.updateLayout()
		return m, nil
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
				m.detail.setNode(node, m.xrefs)
				m.updateLayout()
			}
		}
		return m, nil
	}

	if handlePaneNav(&m.diag, msg) {
		return m, nil
	}

	// Forward to text input
	var cmd tea.Cmd
	m.diag.input, cmd = m.diag.input.Update(msg)
	m.diag.applyFilter()
	return m, cmd
}

func (m model) updateModule(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.module.deactivate()
		m.topPane = topDetail
		m.focus = focusTree
		m.updateLayout()
		return m, nil
	case "enter":
		m.module.toggleExpand()
		return m, nil
	}

	if handlePaneNav(&m.module, msg) {
		return m, nil
	}

	// Forward to text input
	var cmd tea.Cmd
	m.module.input, cmd = m.module.input.Update(msg)
	m.module.applyFilter()
	return m, cmd
}

func (m model) updateTypes(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.typeBrowser.deactivate()
		m.topPane = topDetail
		m.focus = focusTree
		m.updateLayout()
		return m, nil
	case "enter":
		m.typeBrowser.toggleExpand()
		return m, nil
	case "tab":
		m.typeBrowser.cycleTCFilter()
		return m, nil
	}

	if handlePaneNav(&m.typeBrowser, msg) {
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
		m.queryBar.deactivate()
		m.focus = focusTree
		return m, nil
	case "enter":
		qc := m.queryBar.parse()
		if qc == nil {
			// Parse error or empty - stay in query bar (error shown in view)
			if m.queryBar.input.Value() == "" {
				// Empty input, just close
				m.queryBar.deactivate()
				m.focus = focusTree
			}
			return m, nil
		}
		m.queryBar.deactivate()
		m.focus = focusTree
		return m.dispatchQuery(*qc)
	case "tab":
		m.queryBar.tabComplete()
		return m, nil
	}

	// Forward to text input
	var cmd tea.Cmd
	m.queryBar.input, cmd = m.queryBar.input.Update(msg)
	m.queryBar.resetCompletion()
	m.queryBar.err = ""
	return m, cmd
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
				m.crossRefResultByOID(res.OID)
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

func (m model) updateWatch(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		m.watch.lv.CursorDown()
	case "k", "up":
		m.watch.lv.CursorUp()
	case "ctrl+d", "pgdown":
		m.watch.lv.PageDown()
	case "ctrl+u", "pgup":
		m.watch.lv.PageUp()
	case "home":
		m.watch.lv.GoTop()
	case "G", "end":
		m.watch.lv.GoBottom()
	}
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

func (m model) updateColumnPicker(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		cols := m.columnPicker.result()
		m.applyColumnPickerResult(cols)
		m.columnPicker.deactivate()
		m.topPane = topDetail
		m.focus = m.columnPickerReturnFocus()
		m.updateLayout()
		return m, nil
	case " ", "enter":
		m.columnPicker.toggle()
		return m, nil
	case "J":
		m.columnPicker.moveDown()
		return m, nil
	case "K":
		m.columnPicker.moveUp()
		return m, nil
	case "j":
		m.columnPicker.cursorDown()
		return m, nil
	case "k":
		m.columnPicker.cursorUp()
		return m, nil
	}
	if handlePaneNav(&m.columnPicker, msg) {
		return m, nil
	}
	return m, nil
}

// columnPickerReturnFocus determines which focus to return to after the picker closes.
func (m model) columnPickerReturnFocus() focus {
	switch m.bottomPane {
	case bottomWatch:
		return focusWatch
	case bottomTableData:
		return focusResults
	default:
		return focusTree
	}
}

// openColumnPicker activates the column picker for the current table context.
func (m model) openColumnPicker() (tea.Model, tea.Cmd) {
	var tbl *mib.Object
	var prev []columnEntry

	switch m.bottomPane {
	case bottomWatch:
		tbl = m.watch.tbl
		prev = m.watch.tableColumns
	case bottomTableData:
		tbl = m.tableDataObj
		prev = m.tableData.tableColumns
	}

	if tbl == nil {
		return m.setStatusReturn(statusWarn, "No table active")
	}

	cols := tbl.Columns()
	if len(cols) == 0 {
		return m.setStatusReturn(statusWarn, "Table has no columns")
	}

	indexNames := make(map[string]bool)
	if entry := tbl.Entry(); entry != nil {
		indexNames = snmp.IndexNameSet(entry.EffectiveIndexes())
	}

	m.columnPicker.activate(cols, indexNames, prev)
	m.topPane = topDetail // use top-right pane for picker
	m.focus = focusColumnPicker
	m.updateLayout()
	return m, nil
}

// applyColumnPickerResult stores the column entries on the owning model.
func (m *model) applyColumnPickerResult(cols []columnEntry) {
	switch m.bottomPane {
	case bottomWatch:
		m.watch.tableColumns = cols
	case bottomTableData:
		m.tableData.tableColumns = cols
	}
}

func (m model) updateXref(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.xrefPicker.deactivate()
		m.topPane = topDetail
		m.focus = focusTree
		m.updateLayout()
		return m, nil
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
	case "j":
		m.xrefPicker.cursorDown()
		return m, nil
	case "k":
		m.xrefPicker.cursorUp()
		return m, nil
	}
	if handlePaneNav(&m.xrefPicker, msg) {
		return m, nil
	}
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
