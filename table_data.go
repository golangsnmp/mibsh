package main

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// tableDataModel displays live SNMP table data in columnar format.
type tableDataModel struct {
	tableName string     // table name for header
	columns   []string   // column header names
	rows      [][]string // rows[r][c] = formatted value
	indexCols int        // number of leading columns that are index columns

	cursor  int // selected row
	offset  int // first visible row
	hScroll int // horizontal scroll offset (in columns)

	width  int
	height int

	loading bool   // fetch in progress
	err     error  // fetch error
	fetchOp string // "TABLE ifTable" label
}

func newTableDataModel() tableDataModel {
	return tableDataModel{}
}

func (t *tableDataModel) setSize(w, h int) {
	t.width = w
	t.height = h
}

func (t *tableDataModel) setData(tableName string, columns []string, rows [][]string, indexCols int) {
	t.tableName = tableName
	t.columns = columns
	t.rows = rows
	t.indexCols = indexCols
	t.cursor = 0
	t.offset = 0
	t.hScroll = 0
	t.loading = false
	t.err = nil
}

func (t *tableDataModel) setError(err error) {
	t.err = err
	t.loading = false
	t.rows = nil
}

func (t *tableDataModel) setLoading(label string) {
	t.loading = true
	t.err = nil
	t.fetchOp = label
	t.rows = nil
	t.columns = nil
}

// visibleRows returns the number of data rows that fit on screen,
// accounting for header (1) + column headers (1) + separator (1) lines.
func (t *tableDataModel) visibleRows() int {
	v := t.height - tableDataHeaderLines
	if v < 1 {
		return 1
	}
	return v
}

func (t *tableDataModel) ensureVisible() {
	vis := t.visibleRows()
	if t.cursor < t.offset {
		t.offset = t.cursor
	}
	if t.cursor >= t.offset+vis {
		t.offset = t.cursor - vis + 1
	}
}

func (t *tableDataModel) pageDown() {
	vis := t.visibleRows()
	t.cursor += vis
	if t.cursor >= len(t.rows) {
		t.cursor = len(t.rows) - 1
	}
	if t.cursor < 0 {
		t.cursor = 0
	}
	t.ensureVisible()
}

func (t *tableDataModel) pageUp() {
	vis := t.visibleRows()
	t.cursor -= vis
	if t.cursor < 0 {
		t.cursor = 0
	}
	t.ensureVisible()
}

func (t *tableDataModel) scrollRight() {
	if t.hScroll < len(t.columns)-1 {
		t.hScroll++
	}
}

func (t *tableDataModel) scrollLeft() {
	if t.hScroll > 0 {
		t.hScroll--
	}
}

// colWidths computes the display width for each column, based on header
// names and data content. Columns before hScroll are skipped.
func (t *tableDataModel) colWidths() []int {
	if len(t.columns) == 0 {
		return nil
	}
	widths := make([]int, len(t.columns))
	for i, name := range t.columns {
		widths[i] = lipgloss.Width(name)
	}
	for _, row := range t.rows {
		for i, cell := range row {
			if i < len(widths) {
				w := lipgloss.Width(cell)
				if w > widths[i] {
					widths[i] = w
				}
			}
		}
	}
	// Cap each column width
	for i := range widths {
		if widths[i] < 3 {
			widths[i] = 3
		}
		if widths[i] > 60 {
			widths[i] = 60
		}
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

	if len(t.columns) == 0 || len(t.rows) == 0 {
		header := "TABLE"
		if t.tableName != "" {
			header = "TABLE " + t.tableName
		}
		return styles.Header.Info.Render(header) + "\n" +
			styles.EmptyText.Render("(no data)")
	}

	var b strings.Builder

	// Header line
	header := fmt.Sprintf("TABLE %s (%d rows)", t.tableName, len(t.rows))
	if t.hScroll > 0 {
		header += fmt.Sprintf("  [scroll: +%d cols]", t.hScroll)
	}
	b.WriteString(styles.Header.Info.Render(header))
	b.WriteByte('\n')

	allWidths := t.colWidths()

	// Determine which columns fit starting from hScroll
	type visCol struct {
		idx   int
		width int
	}
	var visCols []visCol
	usedW := 2 // left gutter (selection indicator)
	for i := t.hScroll; i < len(allWidths); i++ {
		colW := allWidths[i] + 2 // 2 chars gap between columns
		if usedW+colW > t.width && len(visCols) > 0 {
			break
		}
		visCols = append(visCols, visCol{idx: i, width: allWidths[i]})
		usedW += colW
	}

	if len(visCols) == 0 {
		return b.String()
	}

	// Column headers
	b.WriteString("  ") // gutter
	for j, vc := range visCols {
		name := t.columns[vc.idx]
		padded := fmt.Sprintf("%-*s", vc.width, truncateCell(name, vc.width))
		if vc.idx < t.indexCols {
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
	vis := t.visibleRows()
	end := t.offset + vis
	if end > len(t.rows) {
		end = len(t.rows)
	}

	for i := t.offset; i < end; i++ {
		row := t.rows[i]

		var line strings.Builder
		// Selection gutter
		if i == t.cursor {
			line.WriteString(selectedBorder() + " ")
		} else {
			line.WriteString("  ")
		}

		for j, vc := range visCols {
			cell := ""
			if vc.idx < len(row) {
				cell = row[vc.idx]
			}
			padded := fmt.Sprintf("%-*s", vc.width, truncateCell(cell, vc.width))
			if vc.idx < t.indexCols {
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

	return attachScrollbar(b.String(), vis, len(t.rows), vis, t.offset)
}

// truncate shortens s to maxW characters, adding ellipsis if needed.
func truncateCell(s string, maxW int) string {
	if lipgloss.Width(s) <= maxW {
		return s
	}
	if maxW <= 1 {
		return s[:maxW]
	}
	// Simple rune-based truncation
	runes := []rune(s)
	for i := len(runes) - 1; i > 0; i-- {
		candidate := string(runes[:i]) + "\u2026"
		if lipgloss.Width(candidate) <= maxW {
			return candidate
		}
	}
	return "\u2026"
}
