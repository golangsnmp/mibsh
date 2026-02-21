package main

import (
	"strings"

	"github.com/golangsnmp/gomib/mib"
)

// detailModel renders info for the currently selected node.
type detailModel struct {
	viewportPane
	mib            *mib.Mib
	node           *mib.Node
	devMode        bool
	resultsFocused bool // true when results pane has focus
}

func newDetailModel(m *mib.Mib) detailModel {
	return detailModel{viewportPane: newViewportPane(), mib: m}
}

func (d *detailModel) setNode(node *mib.Node, xrefs xrefMap) {
	d.node = node
	d.vp.SetContent(d.buildContent(xrefs))
	d.vp.GotoTop()
}

func (d *detailModel) setSize(width, height int, xrefs xrefMap) {
	d.viewportPane.setSize(width, height, 1) // reserve 1 for hint line
	// Rebuild content since word-wrap depends on width
	d.vp.SetContent(d.buildContent(xrefs))
}

func (d *detailModel) view(focused bool) string {

	var b strings.Builder

	if d.node == nil {
		b.WriteString(styles.Label.Render("(no selection)"))
		return b.String()
	}

	// Hint line - show navigation keys when focused, shortcuts otherwise
	var hint string
	if focused {
		hint = "j/k:scroll  tab:next pane  esc:back"
	} else if d.resultsFocused {
		hint = "enter:jump to tree  y:copy  x:xrefs"
	} else if d.devMode {
		hint = "y:copy  x:xrefs  vd:diag  vs:schema  vm:modules  vi:detail"
	} else {
		hint = "y:copy  x:xrefs  vd:diag  vs:schema  vm:modules  vi:inspect"
	}
	b.WriteString(styles.StatusText.Render(hint))
	b.WriteByte('\n')

	vpH := d.height - 1 // account for hint line
	if vpH < 1 {
		vpH = 1
	}
	b.WriteString(attachScrollbar(d.vp.View(), vpH, d.vp.TotalLineCount(), d.vp.VisibleLineCount(), d.vp.YOffset()))
	return b.String()
}
