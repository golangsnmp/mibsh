package main

import (
	"image"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/ultraviolet/layout"
)

func (m model) handleMouseMotion(msg tea.MouseMotionMsg) (tea.Model, tea.Cmd) {
	// Track mouse over context menu items
	if m.contextMenu.visible {
		l := m.cachedLayout
		if m.contextMenu.containsPoint(msg.X, msg.Y, l.area) {
			idx := m.contextMenu.itemAtY(msg.Y, l.area)
			if idx >= 0 && idx < len(m.contextMenu.items) && !m.contextMenu.items[idx].isSep && m.contextMenu.items[idx].enabled {
				m.contextMenu.cursor = idx
			}
		}
		return m, nil // suppress tooltip while menu is open
	}

	l := m.cachedLayout
	pt := image.Pt(msg.X, msg.Y)

	if pt.In(l.tree) {
		row := msg.Y - l.tree.Min.Y + m.tree.lv.Offset()
		if row >= 0 && row < m.tree.lv.Len() {
			r := m.tree.lv.Row(row)
			onIcon := isIconClick(l.tree.Min.X, msg.X, r.depth, r.hasKids)
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
		l := m.cachedLayout
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

	l := m.cachedLayout
	pt := image.Pt(msg.X, msg.Y)

	if pt.In(l.tree) {
		m.handleTreeClick(msg, l)
	} else if pt.In(l.rightTop) {
		m.focus = focusDetail
	} else if pt.In(l.rightBot) {
		m.focus = focusResults
		m.handleBottomPaneClick(msg, l)
	}

	return m, nil
}

// handleTreeClick processes a left-click in the tree pane, including
// double-click detection for toggling nodes and returning focus to the tree.
func (m *model) handleTreeClick(msg tea.MouseClickMsg, l appLayout) {
	row := msg.Y - l.tree.Min.Y + m.tree.lv.Offset()
	if row >= 0 && row < m.tree.lv.Len() {
		isDouble := m.treeClicks.isDoubleClick(row, time.Now())

		m.tree.lv.SetCursor(row)

		// Toggle on double-click or single-click on the icon
		r := m.tree.lv.Row(row)
		clickedIcon := isIconClick(l.tree.Min.X, msg.X, r.depth, r.hasKids)
		if isDouble || clickedIcon {
			m.tree.toggle()
		}
		m.syncSelection()
	}
	m.returnFocusToTree()
}

// handleBottomPaneClick processes a left-click in the bottom-right pane,
// dispatching to results or table data depending on the active bottom pane.
func (m *model) handleBottomPaneClick(msg tea.MouseClickMsg, l appLayout) {
	switch m.bottomPane {
	case bottomResults:
		row := msg.Y - l.rightBot.Min.Y - resultHeaderLines + m.results.dataOffset()
		m.results.clickRow(row)

		// Double-click or icon-click to toggle in tree mode
		if m.results.treeMode && row >= 0 && row < m.results.treeLV.Len() {
			isDouble := m.resultClicks.isDoubleClick(row, time.Now())

			r := m.results.treeLV.Row(row)
			clickedIcon := isIconClick(l.rightBot.Min.X, msg.X, r.depth, r.hasKids)
			if isDouble || clickedIcon {
				m.results.toggleTreeNode()
			}
		}
		m.syncResultSelection()
	case bottomTableData:
		row := msg.Y - l.rightBot.Min.Y - tableDataHeaderLines + m.tableData.lv.Offset()
		m.tableData.clickRow(row)
	}
}

// returnFocusToTree deactivates whatever pane currently has focus and
// switches focus back to the tree.
func (m *model) returnFocusToTree() {
	switch m.focus {
	case focusSearch:
		m.search.deactivate()
		m.focus = focusTree
		m.updateLayout()
	case focusDiag, focusModule, focusTypes:
		switch m.focus {
		case focusDiag:
			m.diag.deactivate()
		case focusModule:
			m.module.deactivate()
		case focusTypes:
			m.typeBrowser.deactivate()
		}
		m.topPane = topDetail
		m.focus = focusTree
		m.updateLayout()
	case focusFilter:
		m.filterBar.deactivate()
		m.focus = focusTree
		m.updateLayout()
	case focusQueryBar:
		m.queryBar.deactivate()
		m.focus = focusTree
	case focusResults, focusDetail:
		m.focus = focusTree
	}
}

func (m model) handleMouseWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	if m.contextMenu.visible {
		m.contextMenu.close()
	}

	l := m.cachedLayout
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

func (m model) handleDialogClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if msg.Button != tea.MouseLeft || m.dialog == nil {
		return m, nil
	}

	// Reconstruct the dialog box rect (same logic as drawCentered)
	l := m.cachedLayout
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
