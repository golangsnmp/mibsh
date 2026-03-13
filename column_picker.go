package main

import (
	"fmt"
	"strings"

	"github.com/golangsnmp/gomib/mib"
)

// columnEntry represents a column in the column picker, tracking its name,
// visibility, and whether it is an index column.
type columnEntry struct {
	name    string
	visible bool
	isIndex bool
}

// columnPickerModel displays a navigable list of table columns for
// toggling visibility and reordering. Follows the xrefPickerModel pattern.
type columnPickerModel struct {
	lv      ListView[columnEntry]
	entries []columnEntry
	active  bool
	width   int
	height  int
}

func newColumnPicker() columnPickerModel {
	return columnPickerModel{
		lv: NewListView[columnEntry](2), // header + hint
	}
}

// activate populates the picker from MIB columns. If prev is non-empty and
// matches the same set of column names, the previous state is restored
// (preserving user reordering and visibility). Otherwise all columns start
// visible.
func (cp *columnPickerModel) activate(columns []*mib.Object, indexNames map[string]bool, prev []columnEntry) {
	cp.active = true

	if len(prev) > 0 && columnsMatch(prev, columns) {
		cp.entries = prev
		cp.lv.SetRows(cp.entries)
		cp.lv.GoTop()
		return
	}

	cp.entries = make([]columnEntry, len(columns))
	for i, col := range columns {
		cp.entries[i] = columnEntry{
			name:    col.Name(),
			visible: true,
			isIndex: indexNames[col.Name()],
		}
	}
	cp.lv.SetRows(cp.entries)
	cp.lv.GoTop()
}

func (cp *columnPickerModel) deactivate() {
	cp.active = false
}

func (cp *columnPickerModel) setSize(width, height int) {
	cp.width = width
	cp.height = height
	cp.lv.SetSize(width, height)
}

// toggle flips visibility of the current column. Index columns cannot be hidden.
func (cp *columnPickerModel) toggle() {
	sel := cp.lv.Selected()
	if sel == nil || sel.isIndex {
		return
	}
	idx := cp.lv.Cursor()
	cp.entries[idx].visible = !cp.entries[idx].visible
	cp.lv.SetRows(cp.entries)
}

// moveUp swaps the current entry with the one above and moves the cursor.
func (cp *columnPickerModel) moveUp() {
	idx := cp.lv.Cursor()
	if idx <= 0 {
		return
	}
	cp.entries[idx], cp.entries[idx-1] = cp.entries[idx-1], cp.entries[idx]
	cp.lv.SetRows(cp.entries)
	cp.lv.SetCursor(idx - 1)
}

// moveDown swaps the current entry with the one below and moves the cursor.
func (cp *columnPickerModel) moveDown() {
	idx := cp.lv.Cursor()
	if idx >= len(cp.entries)-1 {
		return
	}
	cp.entries[idx], cp.entries[idx+1] = cp.entries[idx+1], cp.entries[idx]
	cp.lv.SetRows(cp.entries)
	cp.lv.SetCursor(idx + 1)
}

// result returns the current column entries with their visibility and order.
func (cp *columnPickerModel) result() []columnEntry {
	return cp.entries
}

// Cursor delegation: bridges navigablePane interface to ListView.
func (cp *columnPickerModel) cursorUp()   { cp.lv.CursorUp() }
func (cp *columnPickerModel) cursorDown() { cp.lv.CursorDown() }
func (cp *columnPickerModel) goTop()      { cp.lv.GoTop() }
func (cp *columnPickerModel) goBottom()   { cp.lv.GoBottom() }
func (cp *columnPickerModel) pageUp()     { cp.lv.PageUp() }
func (cp *columnPickerModel) pageDown()   { cp.lv.PageDown() }

func (cp *columnPickerModel) view() string {
	if !cp.active {
		return ""
	}

	var b strings.Builder

	// Hint line
	hint := "space:toggle  J/K:move  esc:close"
	b.WriteString(styles.StatusText.Render(hint))
	b.WriteByte('\n')

	// Header
	visible := 0
	for _, e := range cp.entries {
		if e.visible {
			visible++
		}
	}
	header := fmt.Sprintf("Columns (%d/%d visible)", visible, len(cp.entries))
	b.WriteString(styles.Header.Info.Render(header))
	b.WriteByte('\n')

	// List
	if len(cp.entries) == 0 {
		b.WriteString(styles.EmptyText.Render("(no columns)"))
	} else {
		b.WriteString(cp.lv.Render(columnEntryRenderFunc))
	}

	return b.String()
}

func columnEntryRenderFunc(entry columnEntry, _ int, selected bool, width int) string {
	check := "[x] "
	if !entry.visible {
		check = "[ ] "
	}
	if entry.isIndex {
		check = "[x] " // always shown as checked
	}

	name := entry.name
	var styled string
	if entry.isIndex {
		styled = styles.Label.Render(check) + styles.Table.Index.Render(name)
	} else if entry.visible {
		styled = styles.Label.Render(check) + styles.Value.Render(name)
	} else {
		styled = styles.Label.Render(check+name)
	}

	if selected {
		return renderSelectedRow(styled, width)
	}
	return "  " + styled
}

// columnsMatch checks if prev entries have the same column names as columns,
// ignoring order.
func columnsMatch(prev []columnEntry, columns []*mib.Object) bool {
	if len(prev) != len(columns) {
		return false
	}
	names := make(map[string]bool, len(columns))
	for _, col := range columns {
		names[col.Name()] = true
	}
	for _, e := range prev {
		if !names[e.name] {
			return false
		}
	}
	return true
}
