package main

// expandableList provides cursor navigation and expand/collapse for a ListView
// of viewLine items. Rows with modIdx >= 0 are selectable headers; rows with
// modIdx == -1 are non-selectable detail lines. Both typeModel and moduleModel
// embed this to avoid duplicating the same navigation and toggle logic.
type expandableList struct {
	lv       ListView[viewLine]
	expanded map[int]bool
}

func newExpandableList(reserved int) expandableList {
	return expandableList{
		lv:       NewListView[viewLine](reserved),
		expanded: make(map[int]bool),
	}
}

// rebuildViewLines rebuilds the flattened view from item count and callbacks.
// headerFn returns the rendered header text for item at index i.
// detailFn returns expanded detail lines for item at index i.
func (el *expandableList) rebuildViewLines(count int, headerFn func(i int) string, detailFn func(i int) []string) {
	var lines []viewLine
	for i := range count {
		lines = append(lines, viewLine{
			text:   headerFn(i),
			modIdx: i,
		})
		if el.expanded[i] {
			for _, dl := range detailFn(i) {
				lines = append(lines, viewLine{
					text:   dl,
					modIdx: -1,
				})
			}
		}
	}
	el.lv.SetRows(lines)
	el.snapToSelectable()
}

// toggleExpand toggles expand/collapse on the currently selected item.
// Returns true if the state changed (caller should call rebuildViewLines).
func (el *expandableList) toggleExpand() bool {
	sel := el.lv.Selected()
	if sel == nil || sel.modIdx < 0 {
		return false
	}
	el.expanded[sel.modIdx] = !el.expanded[sel.modIdx]
	return true
}

// resetExpanded clears all expanded state.
func (el *expandableList) resetExpanded() {
	el.expanded = make(map[int]bool)
}

// snapToSelectable adjusts the cursor to the nearest selectable (header) row.
func (el *expandableList) snapToSelectable() {
	rows := el.lv.Rows()
	if len(rows) == 0 {
		return
	}
	cursor := el.lv.Cursor()
	if cursor < len(rows) && rows[cursor].modIdx >= 0 {
		return
	}
	for i := cursor; i < len(rows); i++ {
		if rows[i].modIdx >= 0 {
			el.lv.SetCursor(i)
			return
		}
	}
	for i := cursor - 1; i >= 0; i-- {
		if rows[i].modIdx >= 0 {
			el.lv.SetCursor(i)
			return
		}
	}
}

func (el *expandableList) cursorDown() {
	rows := el.lv.Rows()
	cursor := el.lv.Cursor()
	for i := cursor + 1; i < len(rows); i++ {
		if rows[i].modIdx >= 0 {
			el.lv.SetCursor(i)
			return
		}
	}
}

func (el *expandableList) cursorUp() {
	rows := el.lv.Rows()
	cursor := el.lv.Cursor()
	for i := cursor - 1; i >= 0; i-- {
		if rows[i].modIdx >= 0 {
			el.lv.SetCursor(i)
			return
		}
	}
}

func (el *expandableList) cursorBy(n int) {
	if n > 0 {
		for range n {
			el.cursorDown()
		}
	} else {
		for range -n {
			el.cursorUp()
		}
	}
}

func (el *expandableList) pageDown() {
	el.lv.PageDown()
	el.snapToSelectable()
}

func (el *expandableList) pageUp() {
	el.lv.PageUp()
	el.snapToSelectable()
}

func (el *expandableList) goTop() {
	el.lv.GoTop()
	el.snapToSelectable()
}

func (el *expandableList) goBottom() {
	el.lv.GoBottom()
	el.snapToSelectable()
}

// renderViewLineFn is the shared RenderFunc for expandable list view lines.
func renderViewLineFn(vl viewLine, _ int, selected bool, width int) string {
	if selected && vl.modIdx >= 0 {
		return renderSelectedRow(vl.text, width)
	}
	return "  " + vl.text
}
