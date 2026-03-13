package main

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// tableDataModel displays live SNMP table data in columnar format.
type tableDataModel struct {
	tableName    string        // table name for header
	columns      []string      // column header names
	indexCols    int           // number of leading columns that are index columns
	hScroll      int           // horizontal scroll offset (in columns)
	tableColumns []columnEntry // column visibility/ordering from picker

	lv ListView[[]string] // row data, cursor, offset, scrolling

	width int // kept for column-width calculations in view()

	loading bool   // fetch in progress
	err     error  // fetch error
	fetchOp string // "TABLE ifTable" label
}

func newTableDataModel() tableDataModel {
	return tableDataModel{
		lv: NewListView[[]string](tableDataHeaderLines),
	}
}

func (t *tableDataModel) setSize(width, height int) {
	t.width = width
	t.lv.SetSize(width, height)
}

func (t *tableDataModel) setData(tableName string, columns []string, rows [][]string, indexCols int) {
	t.tableName = tableName
	t.columns = columns
	t.indexCols = indexCols
	t.hScroll = 0
	t.loading = false
	t.err = nil
	t.lv.SetRows(rows)
	t.lv.GoTop()
}

func (t *tableDataModel) setError(err error) {
	t.err = err
	t.loading = false
	t.lv.SetRows(nil)
}

func (t *tableDataModel) setLoading(label string) {
	t.loading = true
	t.err = nil
	t.fetchOp = label
	t.lv.SetRows(nil)
	t.columns = nil
	t.tableColumns = nil
}

func (t *tableDataModel) scrollRight() {
	effCount := len(t.effectiveColumns())
	if effCount == 0 {
		effCount = len(t.columns)
	}
	if t.hScroll < effCount-1 {
		t.hScroll++
	}
}

func (t *tableDataModel) scrollLeft() {
	if t.hScroll > 0 {
		t.hScroll--
	}
}

// clickRow sets the cursor to the given row index if in range.
func (t *tableDataModel) clickRow(row int) {
	rows := t.lv.Rows()
	if row >= 0 && row < len(rows) {
		t.lv.SetCursor(row)
	}
}

// selectedRow returns the row data at the cursor, or nil if out of range.
func (t *tableDataModel) selectedRow() []string {
	if sel := t.lv.Selected(); sel != nil {
		return *sel
	}
	return nil
}

// tableDataCol maps a visible column to its source data index.
type tableDataCol struct {
	srcIdx  int    // index into t.columns and row data
	name    string // column header name
	isIndex bool   // whether this is an index column
}

// effectiveColumns returns the visible columns in display order, applying
// tableColumns (from the column picker) if set. Without picker state, returns
// columns in their original order.
func (t *tableDataModel) effectiveColumns() []tableDataCol {
	if len(t.columns) == 0 {
		return nil
	}

	if len(t.tableColumns) > 0 {
		nameToIdx := make(map[string]int, len(t.columns))
		for i, name := range t.columns {
			nameToIdx[name] = i
		}
		var cols []tableDataCol
		for _, ce := range t.tableColumns {
			if !ce.visible {
				continue
			}
			if idx, ok := nameToIdx[ce.name]; ok {
				cols = append(cols, tableDataCol{
					srcIdx:  idx,
					name:    ce.name,
					isIndex: ce.isIndex,
				})
			}
		}
		if len(cols) > 0 {
			return cols
		}
	}

	// Default: all columns in original order
	cols := make([]tableDataCol, len(t.columns))
	for i, name := range t.columns {
		cols[i] = tableDataCol{
			srcIdx:  i,
			name:    name,
			isIndex: i < t.indexCols,
		}
	}
	return cols
}

// colWidths computes the display width for each effective column, based on
// header names and data content.
func (t *tableDataModel) colWidths(effCols []tableDataCol) []int {
	if len(effCols) == 0 {
		return nil
	}
	widths := make([]int, len(effCols))
	for i, col := range effCols {
		widths[i] = lipgloss.Width(col.name)
	}
	for _, row := range t.lv.Rows() {
		for i, col := range effCols {
			if col.srcIdx < len(row) {
				w := lipgloss.Width(row[col.srcIdx])
				if w > widths[i] {
					widths[i] = w
				}
			}
		}
	}
	// Cap each column width
	for i := range widths {
		widths[i] = max(3, min(60, widths[i]))
	}
	return widths
}

func (t *tableDataModel) view() string {
	if t.loading {
		return styles.Header.Info.Render(t.fetchOp) + "\n" +
			styles.EmptyText.Render(IconLoading+" Fetching table data...")
	}

	if t.err != nil {
		header := t.fetchOp
		if header == "" {
			header = "TABLE"
		}
		return styles.Header.Info.Render(header) + "\n" +
			styles.Status.ErrorMsg.Render("Error: "+t.err.Error())
	}

	rows := t.lv.Rows()
	if len(t.columns) == 0 || len(rows) == 0 {
		header := "TABLE"
		if t.tableName != "" {
			header = "TABLE " + t.tableName
		}
		return styles.Header.Info.Render(header) + "\n" +
			styles.EmptyText.Render("(no data)")
	}

	effCols := t.effectiveColumns()
	if len(effCols) == 0 {
		return styles.Header.Info.Render("TABLE "+t.tableName) + "\n" +
			styles.EmptyText.Render("(all columns hidden)")
	}

	var b strings.Builder

	// Header line
	header := fmt.Sprintf("TABLE %s (%d rows)", t.tableName, len(rows))
	if t.hScroll > 0 {
		header += fmt.Sprintf("  [scroll: +%d cols]", t.hScroll)
	}
	b.WriteString(styles.Header.Info.Render(header))
	b.WriteByte('\n')

	allWidths := t.colWidths(effCols)

	// Determine which columns fit starting from hScroll
	type visCol struct {
		effIdx int // index into effCols
		width  int
	}
	var visCols []visCol
	usedW := 2 // left gutter (selection indicator)
	for i := t.hScroll; i < len(allWidths); i++ {
		colW := allWidths[i] + 2 // 2 chars gap between columns
		if usedW+colW > t.width && len(visCols) > 0 {
			break
		}
		visCols = append(visCols, visCol{effIdx: i, width: allWidths[i]})
		usedW += colW
	}

	if len(visCols) == 0 {
		return b.String()
	}

	// Column headers
	b.WriteString("  ") // gutter
	for j, vc := range visCols {
		col := effCols[vc.effIdx]
		padded := fmt.Sprintf("%-*s", vc.width, truncate(col.name, vc.width))
		if col.isIndex {
			b.WriteString(styles.Table.Index.Render(padded))
		} else {
			b.WriteString(styles.Header.Info.Render(padded))
		}
		if j < len(visCols)-1 {
			b.WriteString("  ")
		}
	}
	b.WriteByte('\n')

	// Separator
	b.WriteString("  ") // gutter
	for j, vc := range visCols {
		b.WriteString(styles.Table.Sep.Render(strings.Repeat("\u2500", vc.width)))
		if j < len(visCols)-1 {
			b.WriteString("  ")
		}
	}
	b.WriteByte('\n')

	// Data rows
	vis := t.lv.VisibleRows()
	offset := t.lv.Offset()
	cursor := t.lv.Cursor()
	end := offset + vis
	if end > len(rows) {
		end = len(rows)
	}

	for i := offset; i < end; i++ {
		row := rows[i]

		var line strings.Builder
		// Selection gutter
		if i == cursor {
			line.WriteString(selectedBorder() + " ")
		} else {
			line.WriteString("  ")
		}

		for j, vc := range visCols {
			col := effCols[vc.effIdx]
			cell := ""
			if col.srcIdx < len(row) {
				cell = row[col.srcIdx]
			}
			padded := fmt.Sprintf("%-*s", vc.width, truncate(cell, vc.width))
			if col.isIndex {
				line.WriteString(styles.Table.Index.Render(padded))
			} else {
				line.WriteString(styles.Value.Render(padded))
			}
			if j < len(visCols)-1 {
				line.WriteString("  ")
			}
		}

		b.WriteString(line.String())
		if i < end-1 {
			b.WriteByte('\n')
		}
	}

	return attachScrollbar(b.String(), vis, len(rows), vis, offset)
}
