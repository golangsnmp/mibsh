package main

import (
	"fmt"
	"strings"

	"github.com/golangsnmp/gomib/mib"
	"github.com/golangsnmp/mibsh/internal/snmp"
)

// tableSchemaModel renders a structured SNMP table schema view in the detail pane.
type tableSchemaModel struct {
	viewportPane
	table      *mib.Object // the table object, nil if not in a table
	currentCol string      // name of the column the tree cursor is on, empty if not a column
}

func newTableSchemaModel() tableSchemaModel {
	return tableSchemaModel{viewportPane: newViewportPane()}
}

// setNode resolves the table context from a node. Works for table, row,
// or column nodes. Returns true if a table was found.
func (t *tableSchemaModel) setNode(node *mib.Node) bool {
	obj := node.Object()
	if obj == nil {
		t.table = nil
		t.vp.SetContent(t.buildContent())
		t.vp.GotoTop()
		return false
	}

	tbl, colName := resolveTable(obj, node.Kind())

	if tbl == nil {
		t.table = nil
		t.currentCol = ""
		t.vp.SetContent(t.buildContent())
		t.vp.GotoTop()
		return false
	}

	sameTable := t.table == tbl
	t.table = tbl
	t.currentCol = colName
	t.vp.SetContent(t.buildContent())
	if !sameTable {
		t.vp.GotoTop()
	}
	return true
}

func (t *tableSchemaModel) setSize(width, height int) {
	t.viewportPane.setSize(width, height, 0) // no reserved lines
	t.vp.SetContent(t.buildContent())
}

func (t *tableSchemaModel) view() string {

	var b strings.Builder

	if t.table == nil {
		b.WriteString(styles.Label.Render("(not in a table)"))
		return b.String()
	}

	vpH := t.height
	if vpH < 1 {
		vpH = 1
	}
	b.WriteString(attachScrollbar(t.vp.View(), vpH, t.vp.TotalLineCount(), t.vp.VisibleLineCount(), t.vp.YOffset()))
	return b.String()
}

func (t *tableSchemaModel) buildContent() string {
	if t.table == nil {
		return ""
	}

	var b strings.Builder
	tbl := t.table

	// Header
	writeHeader(&b, tbl.Name())

	// OID
	writeLine(&b, "OID", tbl.OID().String())

	// Module
	if tbl.Module() != nil {
		writeLine(&b, "Module", tbl.Module().Name())
	}

	// Status
	writeLine(&b, "Status", tbl.Status().String())

	// Row entry
	entry := tbl.Entry()
	if entry != nil {
		writeLine(&b, "Entry", entry.Name())

		// Augments
		if entry.Augments() != nil {
			writeLine(&b, "Augments", entry.Augments().Name())
		}
	}

	// Index
	indexes := t.effectiveIndexes()
	if len(indexes) > 0 {
		writeLine(&b, "Index", formatIndexList(indexes))
	}

	// Description
	if tbl.Description() != "" {
		writeDescription(&b, tbl.Description(), t.width-4)
	}

	// Column table
	cols := tbl.Columns()
	if len(cols) > 0 {
		b.WriteByte('\n')
		t.writeColumnTable(&b, cols, indexes)
	}

	return b.String()
}

func (t *tableSchemaModel) effectiveIndexes() []mib.IndexEntry {
	entry := t.table.Entry()
	if entry == nil {
		return nil
	}
	return entry.EffectiveIndexes()
}

func (t *tableSchemaModel) writeColumnTable(b *strings.Builder, cols []*mib.Object, indexes []mib.IndexEntry) {
	indexSet := snmp.IndexNameSet(indexes)

	// Collect column data
	type colData struct {
		marker string
		name   string
		typ    string
		access string
		status string
		desc   string
	}

	rows := make([]colData, 0, len(cols))
	for _, col := range cols {
		marker := "  "
		if indexSet[col.Name()] {
			marker = "* "
		}

		typStr := ""
		if col.Type() != nil {
			typStr = col.Type().Name()
			if typStr == "" {
				typStr = col.Type().Base().String()
			}
			typStr += formatRangeSuffix(col.EffectiveRanges())
			typStr += formatSizeSuffix(col.EffectiveSizes())
		}

		rows = append(rows, colData{
			marker: marker,
			name:   col.Name(),
			typ:    typStr,
			access: col.Access().String(),
			status: col.Status().String(),
			desc:   col.Description(),
		})
	}

	// Calculate column widths
	nameW, typW, accessW := 4, 4, 6 // minimum: "Name", "Type", "Access"
	for _, r := range rows {
		nameW = max(nameW, len(r.name))
		typW = max(typW, len(r.typ))
		accessW = max(accessW, len(r.access))
	}

	// Header
	hdr := fmt.Sprintf("  %-*s  %-*s  %-*s  %s",
		nameW, "Name", typW, "Type", accessW, "Access", "Status")
	b.WriteString(styles.Header.Info.Render(hdr))
	b.WriteByte('\n')

	// Separator
	sep := "  " + strings.Repeat("-", nameW) + "  " +
		strings.Repeat("-", typW) + "  " +
		strings.Repeat("-", accessW) + "  " +
		strings.Repeat("-", 6)
	b.WriteString(styles.Table.Sep.Render(sep))
	b.WriteByte('\n')

	// Rows
	for _, r := range rows {
		line := fmt.Sprintf("%s%-*s  %-*s  %-*s  %s",
			r.marker, nameW, r.name, typW, r.typ, accessW, r.access, r.status)

		switch {
		case r.name == t.currentCol:
			b.WriteString(styles.Table.CurrentCol.Render(line))
		case r.marker == "* ":
			b.WriteString(styles.Table.Index.Render(line))
		default:
			b.WriteString(styles.Value.Render(line))
		}
		b.WriteByte('\n')

		// Show description for the current column
		if r.name == t.currentCol && r.desc != "" {
			desc := normalizeDescription(r.desc)
			b.WriteString(wrapText(desc, t.width-6, "      ", "      "))
			b.WriteString("\n\n")
		}
	}
}
