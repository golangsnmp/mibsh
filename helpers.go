package main

import (
	"fmt"
	"image"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"
	"github.com/golangsnmp/gomib/mib"
)

// viewportPane is an embeddable struct for panes backed by a scrollable
// viewport. It holds the viewport model plus the pane dimensions and provides
// scroll passthrough methods. Embedders call setSize with reserveHeight to
// account for extra chrome (e.g. a hint line) above or below the viewport.
type viewportPane struct {
	vp     viewport.Model
	width  int
	height int
}

func newViewportPane() viewportPane {
	vp := viewport.New()
	vp.MouseWheelEnabled = false
	return viewportPane{vp: vp}
}

// setSize updates the pane dimensions and resizes the viewport.
// reserveHeight lines are subtracted from the viewport height (e.g. for a
// hint line); one column is always reserved for the scrollbar.
func (v *viewportPane) setSize(width, height, reserveHeight int) {
	v.width = width
	v.height = height
	vpH := height - reserveHeight
	if vpH < 1 {
		vpH = 1
	}
	vpW := width - 1 // reserve 1 column for scrollbar
	if vpW < 1 {
		vpW = 1
	}
	v.vp.SetWidth(vpW)
	v.vp.SetHeight(vpH)
}

func (v *viewportPane) scrollDownBy(n int) { v.vp.ScrollDown(n) }
func (v *viewportPane) scrollUpBy(n int)   { v.vp.ScrollUp(n) }

// clickTracker tracks the last click row and time for double-click detection.
type clickTracker struct {
	row int
	at  time.Time
}

// isDoubleClick returns true if this click on the same row is within
// the double-click threshold of the previous click, then updates state.
func (c *clickTracker) isDoubleClick(row int, now time.Time) bool {
	isDouble := row == c.row && now.Sub(c.at) < doubleClickThreshold
	c.row = row
	c.at = now
	return isDouble
}

// newStyledInput creates a textinput with the standard prompt styling applied.
func newStyledInput(prompt string, charLimit int) textinput.Model {
	ti := textinput.New()
	ti.Prompt = prompt
	ti.CharLimit = charLimit
	s := ti.Styles()
	s.Focused.Prompt = styles.Prompt
	s.Blurred.Prompt = styles.Prompt
	ti.SetStyles(s)
	return ti
}

// tabCompleter handles prefix-based tab completion with match cycling.
type tabCompleter struct {
	matches    []string
	matchIdx   int
	lastPrefix string
}

func (tc *tabCompleter) reset() {
	tc.matches = nil
	tc.matchIdx = 0
	tc.lastPrefix = ""
}

// complete returns the next match for the given prefix from candidates.
// When the prefix changes, matches are rebuilt. Returns empty string and
// false if no candidates match.
func (tc *tabCompleter) complete(prefix string, candidates []string) (string, bool) {
	if prefix != tc.lastPrefix {
		tc.lastPrefix = prefix
		tc.matches = tc.matches[:0]
		tc.matchIdx = 0
		prefixLow := strings.ToLower(prefix)
		for _, c := range candidates {
			if strings.HasPrefix(strings.ToLower(c), prefixLow) {
				tc.matches = append(tc.matches, c)
			}
		}
	}
	if len(tc.matches) == 0 {
		return "", false
	}
	result := tc.matches[tc.matchIdx]
	tc.matchIdx = (tc.matchIdx + 1) % len(tc.matches)
	return result, true
}

// nodeEntityProps returns the status and description from the first entity
// attached to a node, checking Object, Notification, Group, Compliance,
// and Capability in order. If no entity is found, returns zero values and false.
func nodeEntityProps(node *mib.Node) (status mib.Status, description string, found bool) {
	if obj := node.Object(); obj != nil {
		return obj.Status(), obj.Description(), true
	}
	if notif := node.Notification(); notif != nil {
		return notif.Status(), notif.Description(), true
	}
	if grp := node.Group(); grp != nil {
		return grp.Status(), grp.Description(), true
	}
	if comp := node.Compliance(); comp != nil {
		return comp.Status(), comp.Description(), true
	}
	if cap := node.Capability(); cap != nil {
		return cap.Status(), cap.Description(), true
	}
	return 0, "", false
}

// selectedBorder returns the rendered thick left-border glyph used as a
// selection indicator in focused rows.
func selectedBorder() string {
	return styles.Tree.FocusBorder.Render(BorderThick)
}

// renderSelectedRow renders a row with the focused selection border indicator.
// It prepends the thick left-border glyph and pads the result to the given width.
func renderSelectedRow(text string, width int) string {
	return padRight(selectedBorder()+" "+text, width)
}

// renderSelectedLine wraps pre-rendered content with the focused/unfocused
// selection border, separator space, and right-padding. In focused mode the
// border and padding use the highlight background; in unfocused mode the
// border is dimmed and padding is plain spaces.
func renderSelectedLine(content string, width int, focused bool) string {
	if !focused {
		border := styles.Tree.UnfocusBorder.Render(BorderThick)
		return padRight(border+" "+content, width)
	}
	bg := styles.Tree.SelectedBg
	selBg := bg.GetBackground()
	border := styles.Tree.FocusBorder.Background(selBg).Render(BorderThick)
	sp := bg.Render(" ")
	return padRightBg(border+sp+content, width, bg)
}

// matchBadge returns a formatted "N matches" or "1 match" string.
func matchBadge(n int) string {
	if n == 1 {
		return "1 match"
	}
	return fmt.Sprintf("%d matches", n)
}

// formatIndexList formats a list of index entries as "[IMPLIED name, name, ...]".
func formatIndexList(indexes []mib.IndexEntry) string {
	names := make([]string, 0, len(indexes))
	for _, idx := range indexes {
		name := "(unknown)"
		if idx.Object != nil {
			name = idx.Object.Name()
		}
		if idx.Implied {
			name = "IMPLIED " + name
		}
		names = append(names, name)
	}
	return "[" + strings.Join(names, ", ") + "]"
}

// clampRect adjusts (x, y) so that a rectangle of size (w, h) fits within area.
// If the rectangle would extend past the right or bottom edge, it shifts left or up.
// If it would extend past the left or top edge, it clamps to the minimum.
func clampRect(x, y, w, h int, area image.Rectangle) (int, int) {
	if x+w > area.Max.X {
		x = area.Max.X - w
	}
	if x < area.Min.X {
		x = area.Min.X
	}
	if y+h > area.Max.Y {
		y = area.Max.Y - h
	}
	if y < area.Min.Y {
		y = area.Min.Y
	}
	return x, y
}

// isIconClick returns true if the x coordinate falls on the expand/collapse
// icon for a tree row at the given depth within a pane starting at baseX.
func isIconClick(baseX, x, depth int, hasKids bool) bool {
	if !hasKids {
		return false
	}
	iconX := baseX + 3 + depth*2
	return x >= iconX && x < iconX+2
}

// truncate shortens s so its visual width fits within maxW, appending a
// unicode ellipsis if truncated. Uses lipgloss.Width for visual-width
// measurement, handling wide and multi-byte characters correctly.
func truncate(s string, maxW int) string {
	if maxW <= 0 || lipgloss.Width(s) <= maxW {
		return s
	}
	if maxW <= 1 {
		return s[:maxW]
	}
	runes := []rune(s)
	for i := len(runes) - 1; i > 0; i-- {
		candidate := string(runes[:i]) + "\u2026"
		if lipgloss.Width(candidate) <= maxW {
			return candidate
		}
	}
	return "\u2026"
}

// resolveTable resolves a table object from a table, row, or column node.
// Returns the table object and the column name (empty for non-column nodes).
func resolveTable(obj *mib.Object, kind mib.Kind) (*mib.Object, string) {
	switch kind {
	case mib.KindTable:
		return obj, ""
	case mib.KindRow:
		return obj.Table(), ""
	case mib.KindColumn:
		return obj.Table(), obj.Name()
	default:
		return nil, ""
	}
}
