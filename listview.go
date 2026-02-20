package main

import "strings"

// RenderFunc renders a single row in a ListView.
// index is the absolute row index, selected indicates the cursor row,
// and width is the available content width (already adjusted for scrollbar).
type RenderFunc[R any] func(row R, index int, selected bool, width int) string

// ListView is a generic scrollable list with cursor, offset, and rendering.
// R is the row data type.
type ListView[R any] struct {
	rows     []R
	cursor   int
	offset   int
	width    int
	height   int
	reserved int // lines not available for rows (headers, etc.)
}

// NewListView creates a ListView with the given number of reserved lines.
func NewListView[R any](reserved int) ListView[R] {
	return ListView[R]{reserved: reserved}
}

// SetRows replaces the row data and clamps the cursor.
func (lv *ListView[R]) SetRows(rows []R) {
	lv.rows = rows
	if lv.cursor >= len(lv.rows) {
		if len(lv.rows) > 0 {
			lv.cursor = len(lv.rows) - 1
		} else {
			lv.cursor = 0
		}
	}
	lv.EnsureVisible()
}

// SetSize sets the total available width and height.
func (lv *ListView[R]) SetSize(width, height int) {
	lv.width = width
	lv.height = height
}

// Len returns the number of rows.
func (lv *ListView[R]) Len() int { return len(lv.rows) }

// Cursor returns the current cursor position.
func (lv *ListView[R]) Cursor() int { return lv.cursor }

// SetCursor sets the cursor position and ensures visibility.
func (lv *ListView[R]) SetCursor(i int) {
	lv.cursor = i
	lv.EnsureVisible()
}

// Offset returns the current scroll offset.
func (lv *ListView[R]) Offset() int { return lv.offset }

// Selected returns a pointer to the row at the cursor, or nil if empty.
func (lv *ListView[R]) Selected() *R {
	if lv.cursor >= 0 && lv.cursor < len(lv.rows) {
		return &lv.rows[lv.cursor]
	}
	return nil
}

// Row returns the row at index i.
func (lv *ListView[R]) Row(i int) R { return lv.rows[i] }

// Rows returns the underlying row slice.
func (lv *ListView[R]) Rows() []R { return lv.rows }

// VisibleRows returns the number of rows that fit in the visible area.
func (lv *ListView[R]) VisibleRows() int {
	v := lv.height - lv.reserved
	if v < 1 {
		return 1
	}
	return v
}

// CursorUp moves the cursor up one row.
func (lv *ListView[R]) CursorUp() {
	if lv.cursor > 0 {
		lv.cursor--
		lv.EnsureVisible()
	}
}

// CursorDown moves the cursor down one row.
func (lv *ListView[R]) CursorDown() {
	if lv.cursor < len(lv.rows)-1 {
		lv.cursor++
		lv.EnsureVisible()
	}
}

// CursorBy moves the cursor by n rows (positive = down, negative = up),
// clamping to valid range.
func (lv *ListView[R]) CursorBy(n int) {
	lv.cursor += n
	if lv.cursor < 0 {
		lv.cursor = 0
	}
	if lv.cursor >= len(lv.rows) {
		lv.cursor = len(lv.rows) - 1
	}
	if lv.cursor < 0 {
		lv.cursor = 0
	}
	lv.EnsureVisible()
}

// PageDown moves the cursor down by one page.
func (lv *ListView[R]) PageDown() {
	lv.cursor += lv.VisibleRows()
	if lv.cursor >= len(lv.rows) {
		lv.cursor = len(lv.rows) - 1
	}
	if lv.cursor < 0 {
		lv.cursor = 0
	}
	lv.EnsureVisible()
}

// PageUp moves the cursor up by one page.
func (lv *ListView[R]) PageUp() {
	lv.cursor -= lv.VisibleRows()
	if lv.cursor < 0 {
		lv.cursor = 0
	}
	lv.EnsureVisible()
}

// GoTop moves the cursor to the first row.
func (lv *ListView[R]) GoTop() {
	lv.cursor = 0
	lv.offset = 0
}

// GoBottom moves the cursor to the last row.
func (lv *ListView[R]) GoBottom() {
	lv.cursor = len(lv.rows) - 1
	if lv.cursor < 0 {
		lv.cursor = 0
	}
	lv.EnsureVisible()
}

// EnsureVisible adjusts the offset so the cursor is within the visible window.
func (lv *ListView[R]) EnsureVisible() {
	vis := lv.VisibleRows()
	if lv.cursor < lv.offset {
		lv.offset = lv.cursor
	}
	if lv.cursor >= lv.offset+vis {
		lv.offset = lv.cursor - vis + 1
	}
	if lv.offset < 0 {
		lv.offset = 0
	}
}

// Render iterates the visible window, calls fn for each row, joins with
// newlines, and attaches a scrollbar when needed.
func (lv *ListView[R]) Render(fn RenderFunc[R]) string {
	if len(lv.rows) == 0 {
		return ""
	}

	vis := lv.VisibleRows()
	needScroll := len(lv.rows) > vis
	contentW := lv.width
	if needScroll {
		contentW = lv.width - 1
	}

	end := lv.offset + vis
	if end > len(lv.rows) {
		end = len(lv.rows)
	}

	var b strings.Builder
	for i := lv.offset; i < end; i++ {
		if i > lv.offset {
			b.WriteByte('\n')
		}
		b.WriteString(fn(lv.rows[i], i, i == lv.cursor, contentW))
	}

	content := b.String()
	if needScroll {
		sb := renderScrollbar(vis, len(lv.rows), vis, lv.offset)
		if sb != "" {
			content = joinScrollbar(content, sb)
		}
	}

	return content
}
