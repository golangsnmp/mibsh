package main

import (
	"strings"

	"charm.land/bubbles/v2/viewport"
	"github.com/golangsnmp/gomib/mib"
)

// detailModel renders info for the currently selected node.
type detailModel struct {
	node           *mib.Node
	viewport       viewport.Model
	xrefs          xrefMap
	width          int
	height         int
	devMode        bool
	resultsFocused bool // true when results pane has focus
}

func newDetailModel() detailModel {
	vp := viewport.New()
	vp.MouseWheelEnabled = false
	return detailModel{viewport: vp}
}

func (d *detailModel) setNode(node *mib.Node) {
	d.node = node
	d.viewport.SetContent(d.buildContent())
	d.viewport.GotoTop()
}

func (d *detailModel) setSize(width, height int) {
	d.width = width
	d.height = height
	vpH := height - 1 // reserve 1 for hint line
	if vpH < 1 {
		vpH = 1
	}
	vpW := width - 1 // reserve 1 column for scrollbar
	if vpW < 1 {
		vpW = 1
	}
	d.viewport.SetWidth(vpW)
	d.viewport.SetHeight(vpH)
	// Rebuild content since word-wrap depends on width
	d.viewport.SetContent(d.buildContent())
}

func (d *detailModel) scrollDown()        { d.viewport.ScrollDown(1) }
func (d *detailModel) scrollUp()          { d.viewport.ScrollUp(1) }
func (d *detailModel) scrollDownBy(n int) { d.viewport.ScrollDown(n) }
func (d *detailModel) scrollUpBy(n int)   { d.viewport.ScrollUp(n) }

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
	b.WriteString(attachScrollbar(d.viewport.View(), vpH, d.viewport.TotalLineCount(), d.viewport.VisibleLineCount(), d.viewport.YOffset()))
	return b.String()
}
