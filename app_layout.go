package main

import (
	"image"

	"github.com/charmbracelet/ultraviolet/layout"
	"github.com/golangsnmp/gomib/mib"
)

func (m *model) generateLayout() appLayout {
	area := image.Rect(0, 0, m.width, m.height)

	// 1-cell horizontal margins (left/right border columns drawn here)
	area.Min.X += 1
	area.Max.X -= 1

	// Header bar (1 row)
	headerArea, rest := layout.SplitVertical(area, layout.Fixed(1))

	// Bottom: hint bar (2 rows) / filter / query / search
	bottomH := 2
	if m.focus == focusSearch {
		bottomH += min(maxSearchVisible+1, (m.height-2)/3)
	}

	mainArea, bottomArea := layout.SplitVertical(rest, layout.Fixed(rest.Dy()-bottomH))

	// Reserve 1 row each for top and bottom border lines
	contentArea := mainArea
	contentArea.Min.Y += 1
	contentArea.Max.Y -= 1

	// Left/right split: tree | border col (1 col) | right pane
	treeAndSep, rightFull := layout.SplitHorizontal(contentArea, layout.Percent(m.treeWidthPct))
	treeRect, sepRect := layout.SplitHorizontal(treeAndSep, layout.Fixed(treeAndSep.Dx()-1))

	// Split right pane into top + separator + bottom when bottom pane is active
	var rightTop, rightSepRect, rightBot image.Rectangle
	if m.bottomPane == bottomNone {
		rightTop = rightFull
		// rightSepRect and rightBot stay zero
	} else {
		minH := 5
		if rightFull.Dy() < minH*2+1 {
			// Too short, give all space to top
			rightTop = rightFull
		} else {
			topPart, remainder := layout.SplitVertical(rightFull, layout.Percent(50))
			rightSepRect, rightBot = layout.SplitVertical(remainder, layout.Fixed(1))
			rightTop = topPart
		}
	}

	return appLayout{
		area:     image.Rect(0, 0, m.width, m.height),
		header:   headerArea,
		tree:     treeRect,
		sep:      sepRect,
		rightTop: rightTop,
		rightSep: rightSepRect,
		rightBot: rightBot,
		bottom:   bottomArea,
	}
}

func (m *model) applyLayout(l appLayout) {
	// Pane styles use Padding(0, 1) = 1 left + 1 right = 2 horizontal cells.
	// Sub-models receive the content-area width (total minus padding).
	const panePad = 2
	m.tree.setSize(max(0, l.tree.Dx()-panePad), l.tree.Dy())

	// Top-right sub-pane components
	m.detail.setSize(max(0, l.rightTop.Dx()-panePad), l.rightTop.Dy())
	m.tableSchema.setSize(max(0, l.rightTop.Dx()-panePad), l.rightTop.Dy())
	m.diag.setSize(max(0, l.rightTop.Dx()-panePad), l.rightTop.Dy())
	m.module.setSize(max(0, l.rightTop.Dx()-panePad), l.rightTop.Dy())
	m.typeBrowser.setSize(max(0, l.rightTop.Dx()-panePad), l.rightTop.Dy())
	m.xrefPicker.setSize(max(0, l.rightTop.Dx()-panePad), l.rightTop.Dy())

	// Bottom-right sub-pane components
	botRect := l.rightBot
	if botRect.Dy() == 0 {
		// When no bottom pane, size using the full right area (for results still showing)
		botRect = l.rightTop
	}
	m.results.setSize(max(0, botRect.Dx()-panePad), botRect.Dy())
	m.tableData.setSize(max(0, botRect.Dx()-panePad), botRect.Dy())
	m.search.setSize(m.width)
	m.filterBar.setSize(m.width)
}

func (m *model) updateLayout() {
	l := m.generateLayout()
	m.applyLayout(l)
}

// scrollTopPaneBy scrolls the top-right pane by n lines (positive = down, negative = up).
func (m *model) scrollTopPaneBy(n int) {
	switch m.topPane {
	case topDiag:
		m.diag.cursorBy(n)
	case topModule:
		m.module.cursorBy(n)
	case topTypes:
		m.typeBrowser.cursorBy(n)
	case topTableSchema:
		if n > 0 {
			m.tableSchema.scrollDownBy(n)
		} else {
			m.tableSchema.scrollUpBy(-n)
		}
	default:
		if n > 0 {
			m.detail.scrollDownBy(n)
		} else {
			m.detail.scrollUpBy(-n)
		}
	}
}

// scrollBottomPaneBy scrolls the bottom-right pane by n lines.
func (m *model) scrollBottomPaneBy(n int) {
	switch m.bottomPane {
	case bottomResults:
		if n > 0 {
			m.results.pageDown()
		} else {
			m.results.pageUp()
		}
		m.syncResultSelection()
	case bottomTableData:
		if n > 0 {
			m.tableData.pageDown()
		} else {
			m.tableData.pageUp()
		}
	}
}

// findModuleNode returns the first node belonging to the named module.
func (m *model) findModuleNode(moduleName string) *mib.Node {
	for node := range m.mib.Nodes() {
		if node.Module() != nil && node.Module().Name() == moduleName {
			return node
		}
	}
	return nil
}
