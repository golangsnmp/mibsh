package main

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"github.com/golangsnmp/gomib/mib"
)

// diagModel is the diagnostics view component.
// It shows a filterable, scrollable list of diagnostics in the detail pane.
type diagModel struct {
	input    textinput.Model
	all      []mib.Diagnostic
	lv       ListView[mib.Diagnostic]
	severity int // -1 = all, 0-6 = specific severity level
	width    int
}

func newDiagModel(m *mib.Mib) diagModel {
	ti := newStyledInput("filter: ", 128)

	dm := diagModel{
		input:    ti,
		all:      m.Diagnostics(),
		severity: -1,
		lv:       NewListView[mib.Diagnostic](2),
	}
	dm.applyFilter()
	return dm
}

func (d *diagModel) activate() {
	d.input.SetValue("")
	d.input.Focus()
	d.severity = -1
	d.lv.SetCursor(0)
	d.applyFilter()
}

func (d *diagModel) deactivate() {
	d.input.Blur()
}

func (d *diagModel) cycleSeverity() {
	d.severity++
	if d.severity > int(mib.SeverityInfo) {
		d.severity = -1
	}
	d.applyFilter()
}

func (d *diagModel) applyFilter() {
	query := strings.ToLower(d.input.Value())
	var filtered []mib.Diagnostic

	for i := range d.all {
		diag := &d.all[i]

		if d.severity >= 0 && int(diag.Severity) != d.severity {
			continue
		}

		if query != "" {
			text := strings.ToLower(diag.Message + " " + diag.Module + " " + diag.Code)
			if !strings.Contains(text, query) {
				continue
			}
		}

		filtered = append(filtered, *diag)
	}

	d.lv.SetRows(filtered)
}

func (d *diagModel) cursorDown()    { d.lv.CursorDown() }
func (d *diagModel) cursorUp()      { d.lv.CursorUp() }
func (d *diagModel) cursorBy(n int) { d.lv.CursorBy(n) }
func (d *diagModel) pageDown()      { d.lv.PageDown() }
func (d *diagModel) pageUp()        { d.lv.PageUp() }
func (d *diagModel) goTop()         { d.lv.GoTop() }
func (d *diagModel) goBottom()      { d.lv.GoBottom() }

func (d *diagModel) setSize(width, height int) {
	d.width = width
	d.lv.SetSize(width, height)
}

func (d *diagModel) severityLabel() string {
	if d.severity < 0 {
		return "all"
	}
	return mib.Severity(d.severity).String()
}

func (d *diagModel) selectedDiag() *mib.Diagnostic {
	return d.lv.Selected()
}

func (d *diagModel) view() string {

	var b strings.Builder

	// Line 1: filter input
	b.WriteString(d.input.View())
	b.WriteByte('\n')

	// Line 2: status bar
	status := fmt.Sprintf("severity: %s  (%d/%d)  [tab] cycle",
		d.severityLabel(), d.lv.Len(), len(d.all))
	b.WriteString(styles.StatusText.Render(status))
	b.WriteByte('\n')

	// Diagnostic list
	if d.lv.Len() == 0 {
		b.WriteString(styles.EmptyText.Render("(no diagnostics)"))
	} else {
		b.WriteString(d.lv.Render(d.renderDiagRow))
	}

	return b.String()
}

func (d *diagModel) renderDiagRow(diag mib.Diagnostic, _ int, selected bool, width int) string {
	line := d.renderDiag(diag)
	if selected {
		return renderSelectedRow(line, width)
	}
	return "  " + line
}

func (d *diagModel) renderDiag(diag mib.Diagnostic) string {
	sevText := "[" + diag.Severity.String() + "]"
	sev := padRight(diagSeverityStyle(diag.Severity).Render(sevText), 9)

	var loc string
	if diag.Module != "" {
		loc = diag.Module
		if diag.Line > 0 {
			loc += fmt.Sprintf(":%d", diag.Line)
		}
		loc += ": "
	}

	msg := loc + diag.Message
	if diag.Code != "" {
		msg += " (" + diag.Code + ")"
	}

	return sev + " " + styles.Value.Render(msg)
}
