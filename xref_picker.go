package main

import (
	"fmt"
	"strings"

	"github.com/golangsnmp/gomib/mib"
)

// xrefPickerModel displays a navigable list of cross-references for a node.
type xrefPickerModel struct {
	lv       ListView[xref]
	mib      *mib.Mib
	nodeName string
	active   bool
	width    int
	height   int
}

func newXrefPicker(m *mib.Mib) xrefPickerModel {
	return xrefPickerModel{
		lv:  NewListView[xref](2), // 2 reserved lines: header + hint
		mib: m,
	}
}

func (x *xrefPickerModel) activate(nodeName string, refs []xref) {
	x.nodeName = nodeName
	x.active = true
	x.lv.SetRows(refs)
	x.lv.GoTop()
}

func (x *xrefPickerModel) deactivate() {
	x.active = false
	x.nodeName = ""
	x.lv.SetRows(nil)
}

func (x *xrefPickerModel) setSize(width, height int) {
	x.width = width
	x.height = height
	x.lv.SetSize(width, height)
}

func (x *xrefPickerModel) selectedXref() *xref {
	return x.lv.Selected()
}

// navigablePane implementation

func (x *xrefPickerModel) cursorUp()   { x.lv.CursorUp() }
func (x *xrefPickerModel) cursorDown() { x.lv.CursorDown() }
func (x *xrefPickerModel) goTop()      { x.lv.GoTop() }
func (x *xrefPickerModel) goBottom()   { x.lv.GoBottom() }
func (x *xrefPickerModel) pageUp()     { x.lv.PageUp() }
func (x *xrefPickerModel) pageDown()   { x.lv.PageDown() }

func (x *xrefPickerModel) view() string {
	if !x.active {
		return ""
	}

	var b strings.Builder

	// Hint line
	hint := "enter:jump  esc:back  bksp:prev"
	b.WriteString(styles.StatusText.Render(hint))
	b.WriteByte('\n')

	// Header
	header := fmt.Sprintf("Cross-references for %s (%d)", x.nodeName, x.lv.Len())
	b.WriteString(styles.Header.Info.Render(header))
	b.WriteByte('\n')

	// List
	if x.lv.Len() == 0 {
		b.WriteString(styles.EmptyText.Render("(no cross-references)"))
	} else {
		b.WriteString(x.lv.Render(xrefRenderFunc))
	}

	return b.String()
}

func xrefRenderFunc(ref xref, _ int, selected bool, width int) string {
	var kindLabel string
	switch ref.kind {
	case xrefGroup:
		kindLabel = "group"
	case xrefNotification:
		kindLabel = "notification"
	case xrefCompliance:
		kindLabel = "compliance"
	case xrefIndex:
		kindLabel = "index of"
	}

	styled := styles.Label.Render(kindLabel+" ") + styles.Value.Render(ref.name)
	if ref.via != "" {
		styled += styles.Label.Render(" (" + ref.via + ")")
	}

	if selected {
		return renderSelectedRow(styled, width)
	}
	return "  " + styled
}
