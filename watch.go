package main

import (
	"fmt"
	"math"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/golangsnmp/gomib/mib"
	"github.com/golangsnmp/mibsh/internal/snmp"
	"github.com/gosnmp/gosnmp"
)

const (
	watchDefaultInterval = 10 * time.Second
	watchMinInterval     = 5 * time.Second
	watchMaxInterval     = 120 * time.Second
	watchIntervalStep    = 5 * time.Second
	watchHeaderLines     = 3 // header + column headers + separator
)

// watchSnapshot holds numeric values keyed by OID string for one poll cycle.
type watchSnapshot map[string]float64

// watchEntry holds the formatted data for a single OID from the latest poll.
type watchEntry struct {
	oid      string
	name     string
	typeName string
	value    string
	pduType  gosnmp.Asn1BER
	delta    string // "-" if first poll or non-numeric
	rate     string // "-" if first poll or non-numeric
	changed  bool   // non-numeric value changed from previous
}

// watchModel manages periodic SNMP polling with diff display.
type watchModel struct {
	active   bool
	rootOID  string // OID being polled (dotted string)
	rootName string // display name of the root node
	node     *mib.Node
	interval time.Duration
	pollSeq  uint64 // monotonic counter, incremented on start/stop
	pollNum  int    // poll count for display
	polling  bool   // true while a poll is in flight

	prev    watchSnapshot
	curr    watchSnapshot
	prevStr map[string]string // previous formatted values for change detection
	entries []watchEntry

	// Table mode fields
	isTable      bool
	tbl          *mib.Object
	indexCols    int
	tableColumns []columnEntry // column visibility/ordering from picker

	lv    ListView[watchEntry]
	width int

	hScroll int // horizontal scroll offset for table mode
}

func newWatchModel() watchModel {
	return watchModel{
		interval: watchDefaultInterval,
		lv:       NewListView[watchEntry](watchHeaderLines),
	}
}

// start begins watching the given node's OID.
func (w *watchModel) start(node *mib.Node, m *mib.Mib) {
	w.active = true
	w.node = node
	w.rootOID = node.OID().String()
	w.rootName = node.Name()
	w.pollSeq++
	w.pollNum = 0
	w.polling = false
	w.prev = nil
	w.curr = nil
	w.prevStr = nil
	w.entries = nil
	w.hScroll = 0
	w.lv.SetRows(nil)

	// Detect table mode
	w.isTable = false
	w.tbl = nil
	w.indexCols = 0
	w.tableColumns = nil
	if obj := node.Object(); obj != nil {
		tbl, _ := resolveTable(obj, node.Kind())
		if tbl != nil {
			w.isTable = true
			w.tbl = tbl
			if entry := tbl.Entry(); entry != nil {
				iset := snmp.IndexNameSet(entry.EffectiveIndexes())
				for _, col := range tbl.Columns() {
					if iset[col.Name()] {
						w.indexCols++
					}
				}
			}
		}
	}
}

// stop ends the watch and invalidates any in-flight polls or ticks.
func (w *watchModel) stop() {
	w.active = false
	w.pollSeq++
	w.polling = false
}

// adjustInterval changes the poll interval by delta, clamped to valid range.
func (w *watchModel) adjustInterval(delta time.Duration) {
	w.interval += delta
	if w.interval < watchMinInterval {
		w.interval = watchMinInterval
	}
	if w.interval > watchMaxInterval {
		w.interval = watchMaxInterval
	}
}

// handlePoll processes a completed poll result, computing deltas and rates.
func (w *watchModel) handlePoll(pdus []gosnmp.SnmpPDU, m *mib.Mib, elapsed time.Duration) {
	w.pollNum++
	w.polling = false

	// Rotate snapshots
	w.prev = w.curr
	w.curr = make(watchSnapshot, len(pdus))
	oldStr := w.prevStr
	w.prevStr = make(map[string]string, len(pdus))

	secs := elapsed.Seconds()
	if secs <= 0 {
		secs = 1
	}

	entries := make([]watchEntry, 0, len(pdus))
	for _, pdu := range pdus {
		r := snmp.FormatPDUToResult(pdu, m)
		w.prevStr[pdu.Name] = r.Value

		e := watchEntry{
			oid:      pdu.Name,
			name:     r.Name,
			typeName: r.TypeName,
			value:    r.Value,
			pduType:  pdu.Type,
			delta:    "-",
			rate:     "-",
		}

		if v, ok := snmp.ExtractNumeric(pdu); ok {
			w.curr[pdu.Name] = v

			if w.prev != nil {
				if prevVal, hasPrev := w.prev[pdu.Name]; hasPrev {
					d := computeDelta(prevVal, v, pdu.Type)
					e.delta = formatDelta(d)
					e.rate = formatRate(d, secs)
				}
			}
		} else {
			// Non-numeric: detect value change
			if oldStr != nil {
				if old, ok := oldStr[pdu.Name]; ok && old != r.Value {
					e.changed = true
				}
			}
		}

		entries = append(entries, e)
	}

	w.entries = entries
	w.lv.SetRows(entries)
}

func (w *watchModel) setSize(width, height int) {
	w.width = width
	w.lv.SetSize(width, height)
}

func (w *watchModel) scrollRight() {
	w.hScroll++
}

func (w *watchModel) scrollLeft() {
	if w.hScroll > 0 {
		w.hScroll--
	}
}

func (w *watchModel) clickRow(row int) {
	if row >= 0 && row < w.lv.Len() {
		w.lv.SetCursor(row)
	}
}

// startPollCmd returns a tea.Cmd to begin a poll if conditions are met.
func (w *watchModel) startPollCmd(sess *snmp.Session) tea.Cmd {
	if !w.active || w.polling {
		return nil
	}
	w.polling = true
	return snmp.WatchPollCmd(sess, w.rootOID, w.pollSeq)
}

// scheduleNextTick returns a command to schedule the next poll tick.
func (w *watchModel) scheduleNextTick() tea.Cmd {
	if !w.active {
		return nil
	}
	return snmp.WatchTickCmd(w.interval, w.pollSeq)
}

// view renders the watch pane content.
func (w *watchModel) view() string {
	if !w.active {
		return styles.EmptyText.Render("(no watch active)")
	}

	if w.pollNum == 0 {
		header := fmt.Sprintf("WATCH %s (%s) | polling...", w.rootName, formatInterval(w.interval))
		return styles.Header.Info.Render(header) + "\n" +
			styles.EmptyText.Render(IconLoading+" Waiting for first poll...")
	}

	if w.isTable {
		return w.viewTable()
	}
	return w.viewFlat()
}

// viewFlat renders the non-table watch display.
func (w *watchModel) viewFlat() string {
	var b strings.Builder

	// Header
	header := fmt.Sprintf("WATCH %s (%s) | %d values | poll #%d",
		w.rootName, formatInterval(w.interval), len(w.entries), w.pollNum)
	b.WriteString(styles.Header.Info.Render(header))
	b.WriteByte('\n')

	// Column headers
	typeW := 12
	nameW := 24
	valW := 20
	deltaW := 12
	rateW := 12

	// Compute widths from data
	for _, e := range w.entries {
		if lw := lipgloss.Width(e.typeName); lw > typeW {
			typeW = lw
		}
		if lw := lipgloss.Width(e.name); lw > nameW {
			nameW = lw
		}
		if lw := lipgloss.Width(e.value); lw > valW {
			valW = lw
		}
	}
	typeW = min(typeW, 16)
	nameW = min(nameW, 40)
	valW = min(valW, 40)

	hdr := fmt.Sprintf("  %-*s  %-*s  %-*s  %-*s  %-*s",
		typeW, "TYPE", nameW, "NAME", valW, "VALUE", deltaW, "delta", rateW, "/s")
	b.WriteString(styles.Header.Info.Render(hdr))
	b.WriteByte('\n')

	// Separator
	sepW := typeW + nameW + valW + deltaW + rateW + 10
	b.WriteString("  " + styles.Table.Sep.Render(strings.Repeat("\u2500", min(sepW, w.width-2))))
	b.WriteByte('\n')

	// Data rows
	vis := w.lv.VisibleRows()
	offset := w.lv.Offset()
	cursor := w.lv.Cursor()
	end := offset + vis
	if end > len(w.entries) {
		end = len(w.entries)
	}

	for i := offset; i < end; i++ {
		e := w.entries[i]

		var line strings.Builder
		if i == cursor {
			line.WriteString(selectedBorder() + " ")
		} else {
			line.WriteString("  ")
		}

		line.WriteString(styles.Label.Render(fmt.Sprintf("%-*s", typeW, truncate(e.typeName, typeW))))
		line.WriteString("  ")
		line.WriteString(styles.Value.Render(fmt.Sprintf("%-*s", nameW, truncate(e.name, nameW))))
		line.WriteString("  ")

		valStr := fmt.Sprintf("%-*s", valW, truncate(e.value, valW))
		if e.changed {
			line.WriteString(lipgloss.NewStyle().Foreground(palette.Yellow).Render(valStr))
		} else {
			line.WriteString(styles.Value.Render(valStr))
		}
		line.WriteString("  ")

		line.WriteString(renderDelta(e.delta, deltaW))
		line.WriteString("  ")
		line.WriteString(renderRate(e.rate, rateW))

		b.WriteString(line.String())
		if i < end-1 {
			b.WriteByte('\n')
		}
	}

	return attachScrollbar(b.String(), vis, len(w.entries), vis, offset)
}

// viewTable renders the table-mode watch display with inline delta/rate columns.
func (w *watchModel) viewTable() string {
	var b strings.Builder

	// Header
	header := fmt.Sprintf("WATCH %s (%s) | %d rows | poll #%d",
		w.rootName, formatInterval(w.interval), w.countRows(), w.pollNum)
	if w.hScroll > 0 {
		header += fmt.Sprintf("  [scroll: +%d cols]", w.hScroll)
	}
	b.WriteString(styles.Header.Info.Render(header))
	b.WriteByte('\n')

	// Build table structure from entries
	rows := w.buildTableRows()
	if len(rows) == 0 || len(rows[0]) == 0 {
		b.WriteString(styles.EmptyText.Render("(no data)"))
		return b.String()
	}

	cols := rows[0]
	colCount := len(cols)

	// Compute column widths
	widths := make([]int, colCount)
	for i, col := range cols {
		widths[i] = lipgloss.Width(col.header)
	}
	for _, row := range rows {
		for i, col := range row {
			if i < colCount {
				if w := lipgloss.Width(col.value); w > widths[i] {
					widths[i] = w
				}
			}
		}
	}
	for i := range widths {
		widths[i] = max(3, min(60, widths[i]))
	}

	// Determine visible columns starting from hScroll
	type visCol struct {
		idx   int
		width int
	}
	var visCols []visCol
	usedW := 2 // selection gutter
	logicalCol := 0
	for i := 0; i < colCount; i++ {
		colW := widths[i] + 2
		if logicalCol < w.hScroll {
			logicalCol++
			continue
		}
		if usedW+colW > w.width && len(visCols) > 0 {
			break
		}
		visCols = append(visCols, visCol{idx: i, width: widths[i]})
		usedW += colW
		logicalCol++
	}

	if len(visCols) == 0 {
		return b.String()
	}

	// Column headers
	b.WriteString("  ")
	for j, vc := range visCols {
		col := cols[vc.idx]
		padded := fmt.Sprintf("%-*s", vc.width, truncate(col.header, vc.width))
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
	b.WriteString("  ")
	for j, vc := range visCols {
		b.WriteString(styles.Table.Sep.Render(strings.Repeat("\u2500", vc.width)))
		if j < len(visCols)-1 {
			b.WriteString("  ")
		}
	}
	b.WriteByte('\n')

	// Data rows
	vis := w.lv.VisibleRows()
	offset := w.lv.Offset()
	cursor := w.lv.Cursor()

	// In table mode, we need to compute which "table rows" are visible.
	// Each table row may correspond to multiple entries. For simplicity,
	// we use entries as rows and show the relevant columns per entry.
	// However, we'll map entries to table rows via suffix grouping.
	end := offset + vis
	if end > len(rows) {
		end = len(rows)
	}

	for i := offset; i < end; i++ {
		row := rows[i]

		var line strings.Builder
		if i == cursor {
			line.WriteString(selectedBorder() + " ")
		} else {
			line.WriteString("  ")
		}

		for j, vc := range visCols {
			cell := ""
			if vc.idx < len(row) {
				cell = row[vc.idx].value
			}
			padded := fmt.Sprintf("%-*s", vc.width, truncate(cell, vc.width))
			col := cols[vc.idx]
			if col.isIndex {
				line.WriteString(styles.Table.Index.Render(padded))
			} else if vc.idx < len(row) && row[vc.idx].changed {
				line.WriteString(lipgloss.NewStyle().Foreground(palette.Yellow).Render(padded))
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

// tableCell holds data for a single cell in the table view.
type tableCell struct {
	header  string
	value   string
	isIndex bool
	changed bool
}

// countRows counts unique row suffixes in the entries.
func (w *watchModel) countRows() int {
	if w.tbl == nil {
		return len(w.entries)
	}
	seen := make(map[string]bool)
	cols := w.tbl.Columns()
	m := w.node
	_ = m
	for _, e := range w.entries {
		oid, err := mib.ParseOID(e.oid)
		if err != nil {
			continue
		}
		// Find which column this belongs to
		for _, col := range cols {
			colOID := col.OID()
			if len(oid) > len(colOID) && oid[:len(colOID)].Equal(colOID) {
				suffix := oid[len(colOID):].String()
				seen[suffix] = true
				break
			}
		}
	}
	if len(seen) == 0 {
		return len(w.entries)
	}
	return len(seen)
}

// buildTableRows organizes flat entries into table rows with inline delta/rate
// sub-columns for numeric data columns.
func (w *watchModel) buildTableRows() [][]tableCell {
	if w.tbl == nil || len(w.entries) == 0 {
		return nil
	}

	cols := w.tbl.Columns()
	if len(cols) == 0 {
		return nil
	}

	// Build column index
	type colInfo struct {
		idx     int
		name    string
		colOID  mib.OID
		isIndex bool
		numeric bool
	}
	colMap := make(map[string]*colInfo, len(cols))
	colList := make([]*colInfo, len(cols))
	indexNames := make(map[string]bool)
	if entry := w.tbl.Entry(); entry != nil {
		indexNames = snmp.IndexNameSet(entry.EffectiveIndexes())
	}
	for i, col := range cols {
		numeric := false
		if t := col.Type(); t != nil {
			numeric = isNumericBase(t.EffectiveBase())
		}
		ci := &colInfo{
			idx:     i,
			name:    col.Name(),
			colOID:  col.OID(),
			isIndex: indexNames[col.Name()],
			numeric: numeric,
		}
		colMap[col.OID().String()] = ci
		colList[i] = ci
	}

	// Apply column visibility and ordering from picker
	if len(w.tableColumns) > 0 {
		byName := make(map[string]*colInfo, len(colList))
		for _, ci := range colList {
			byName[ci.name] = ci
		}
		ordered := make([]*colInfo, 0, len(w.tableColumns))
		for _, ce := range w.tableColumns {
			if !ce.visible {
				continue
			}
			if ci, ok := byName[ce.name]; ok {
				ordered = append(ordered, ci)
			}
		}
		if len(ordered) > 0 {
			colList = ordered
		}
	}

	// Build headers: for each column, add the value column, and for numeric
	// non-index columns also add delta and /s sub-columns
	var headers []tableCell
	colExpansion := make([]int, len(cols)) // how many output columns per source column
	for _, ci := range colList {
		headers = append(headers, tableCell{header: ci.name, isIndex: ci.isIndex})
		colExpansion[ci.idx] = 1
		if !ci.isIndex && ci.numeric {
			headers = append(headers, tableCell{header: "delta"})
			headers = append(headers, tableCell{header: "/s"})
			colExpansion[ci.idx] = 3
		}
	}

	// Group entries by suffix
	type rowCell struct {
		value   string
		delta   string
		rate    string
		changed bool
	}
	type rowData struct {
		suffix string
		cells  []rowCell
	}
	rowMap := make(map[string]*rowData)
	var rowOrder []string

	for _, e := range w.entries {
		oid, err := mib.ParseOID(e.oid)
		if err != nil {
			continue
		}

		// Find column
		var ci *colInfo
		var suffix string
		for _, c := range colList {
			if len(oid) > len(c.colOID) && oid[:len(c.colOID)].Equal(c.colOID) {
				ci = c
				suffix = oid[len(c.colOID):].String()
				break
			}
		}
		if ci == nil {
			continue
		}

		rd, exists := rowMap[suffix]
		if !exists {
			rd = &rowData{
				suffix: suffix,
				cells:  make([]rowCell, len(cols)),
			}
			rowMap[suffix] = rd
			rowOrder = append(rowOrder, suffix)
		}

		rd.cells[ci.idx] = rowCell{
			value:   e.value,
			delta:   e.delta,
			rate:    e.rate,
			changed: e.changed,
		}
	}

	// Build output rows
	result := make([][]tableCell, 0, len(rowOrder))
	for _, suffix := range rowOrder {
		rd := rowMap[suffix]
		var row []tableCell
		for _, ci := range colList {
			cell := rd.cells[ci.idx]
			val := cell.value
			if val == "" {
				val = "-"
			}
			row = append(row, tableCell{
				value:   val,
				isIndex: ci.isIndex,
				changed: cell.changed,
			})
			if !ci.isIndex && ci.numeric {
				row = append(row, tableCell{value: cell.delta})
				row = append(row, tableCell{value: cell.rate})
			}
		}
		result = append(result, row)
	}

	// Set headers on the first row's column positions (we return headers separately via a trick)
	// Actually, return rows where first "row" is headers
	// The caller handles this by using cols[0] for headers
	// Let's just pre-pend headers as metadata

	// Store headers in the result structure by using column 0's header field
	for i := range result {
		for j := range result[i] {
			if j < len(headers) {
				result[i][j].header = headers[j].header
				if headers[j].isIndex {
					result[i][j].isIndex = true
				}
			}
		}
	}

	// Update the ListView with correct row count for table mode
	if len(result) != w.lv.Len() {
		w.lv.SetRows(make([]watchEntry, len(result)))
	}

	return result
}

// isNumericBase returns true for base types that produce numeric SNMP values
// suitable for delta/rate computation.
func isNumericBase(b mib.BaseType) bool {
	switch b {
	case mib.BaseInteger32, mib.BaseUnsigned32, mib.BaseCounter32,
		mib.BaseCounter64, mib.BaseGauge32, mib.BaseTimeTicks:
		return true
	}
	return false
}

// --- Delta/rate computation ---

// computeDelta computes the difference between previous and current values,
// handling counter wraps for Counter32 and Counter64 types.
func computeDelta(prev, curr float64, pduType gosnmp.Asn1BER) float64 {
	d := curr - prev
	if d >= 0 {
		return d
	}

	// Counter wrap detection
	switch pduType {
	case gosnmp.Counter32:
		return (math.MaxUint32 - prev) + curr + 1
	case gosnmp.Counter64:
		return (math.MaxUint64 - prev) + curr + 1
	default:
		// For non-counter types, negative delta is just negative
		return d
	}
}

func formatDelta(d float64) string {
	if d == 0 {
		return "0"
	}
	if d == math.Trunc(d) && math.Abs(d) < 1e15 {
		return fmt.Sprintf("%.0f", d)
	}
	return fmt.Sprintf("%.1f", d)
}

func formatRate(d, secs float64) string {
	r := d / secs
	if r == 0 {
		return "0"
	}
	if r >= 1000 {
		return fmt.Sprintf("%.0f", r)
	}
	return fmt.Sprintf("%.1f", r)
}

func formatInterval(d time.Duration) string {
	s := int(d.Seconds())
	return fmt.Sprintf("%ds", s)
}

// renderDelta renders a delta value with appropriate styling and width.
func renderDelta(s string, width int) string {
	padded := fmt.Sprintf("%-*s", width, s)
	if s == "-" || s == "0" {
		return styles.Label.Render(padded)
	}
	return lipgloss.NewStyle().Foreground(palette.Cyan).Render(padded)
}

// renderRate renders a rate value with appropriate styling and width.
func renderRate(s string, width int) string {
	padded := fmt.Sprintf("%-*s", width, s)
	if s == "-" || s == "0" {
		return styles.Label.Render(padded)
	}
	return lipgloss.NewStyle().Foreground(palette.Green).Render(padded)
}
