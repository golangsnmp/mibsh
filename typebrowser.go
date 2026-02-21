package main

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
	"github.com/golangsnmp/gomib/mib"
)

// typeModel is the type/TC browser component.
// It shows a filterable, scrollable list of type definitions in the detail pane.
type typeModel struct {
	expandableList
	input    textinput.Model
	all      []*mib.Type
	filtered []*mib.Type
	width    int
	height   int
	showTC   int // 0=all, 1=TC only, 2=non-TC only
}

func newTypeModel(m *mib.Mib) typeModel {
	ti := newStyledInput("filter: ", 128)

	types := m.Types()
	slices.SortFunc(types, func(a, b *mib.Type) int {
		return cmp.Compare(a.Name(), b.Name())
	})

	tm := typeModel{
		expandableList: newExpandableList(2),
		input:          ti,
		all:            types,
	}
	tm.applyFilter()
	return tm
}

func (tm *typeModel) activate() {
	tm.input.SetValue("")
	tm.input.Focus()
	tm.resetExpanded()
	tm.showTC = 0
	tm.applyFilter()
}

func (tm *typeModel) deactivate() {
	tm.input.Blur()
}

func (tm *typeModel) applyFilter() {
	query := strings.ToLower(tm.input.Value())
	tm.filtered = tm.filtered[:0]

	for _, t := range tm.all {
		// TC filter
		switch tm.showTC {
		case 1:
			if !t.IsTextualConvention() {
				continue
			}
		case 2:
			if t.IsTextualConvention() {
				continue
			}
		}
		if query != "" {
			text := strings.ToLower(t.Name())
			if !strings.Contains(text, query) {
				continue
			}
		}
		tm.filtered = append(tm.filtered, t)
	}

	tm.rebuildViewLines(len(tm.filtered),
		func(i int) string { return tm.renderTypeLine(tm.filtered[i]) },
		func(i int) []string { return tm.renderTypeDetail(tm.filtered[i]) },
	)
}

// toggleExpand toggles expand/collapse on the currently selected type.
func (tm *typeModel) toggleExpand() {
	if tm.expandableList.toggleExpand() {
		tm.rebuildViewLines(len(tm.filtered),
			func(i int) string { return tm.renderTypeLine(tm.filtered[i]) },
			func(i int) []string { return tm.renderTypeDetail(tm.filtered[i]) },
		)
	}
}

// cycleTCFilter cycles the TC filter mode: all -> TC only -> non-TC -> all.
func (tm *typeModel) cycleTCFilter() {
	tm.showTC = (tm.showTC + 1) % 3
	tm.resetExpanded()
	tm.applyFilter()
}

func (tm *typeModel) setSize(width, height int) {
	tm.width = width
	tm.height = height
	tm.lv.SetSize(width, height)
}

func (tm *typeModel) view() string {
	var b strings.Builder

	// Line 1: filter input
	b.WriteString(tm.input.View())
	b.WriteByte('\n')

	// Line 2: status
	tcLabel := "all"
	switch tm.showTC {
	case 1:
		tcLabel = "TC only"
	case 2:
		tcLabel = "non-TC"
	}
	status := fmt.Sprintf("types: %d/%d  [tab] %s  [enter] expand", len(tm.filtered), len(tm.all), tcLabel)
	b.WriteString(styles.StatusText.Render(status))
	b.WriteByte('\n')

	// Type list
	if tm.lv.Len() == 0 {
		b.WriteString(styles.EmptyText.Render("(no types)"))
		return b.String()
	}

	b.WriteString(tm.lv.Render(renderViewLineFn))

	return b.String()
}

func (tm *typeModel) renderTypeLine(t *mib.Type) string {
	tcBadge := lipgloss.NewStyle().Foreground(palette.Green)

	var parts []string
	parts = append(parts, styles.Value.Render(t.Name()))
	parts = append(parts, styles.Label.Render(t.EffectiveBase().String()))
	if t.IsTextualConvention() {
		parts = append(parts, tcBadge.Render("[TC]"))
	}
	if t.Module() != nil {
		parts = append(parts, styles.Label.Render(t.Module().Name()))
	}

	return strings.Join(parts, "  ")
}

func (tm *typeModel) renderTypeDetail(t *mib.Type) []string {
	var lines []string

	// Parent chain
	if t.Parent() != nil {
		var chain []string
		for cur := t.Parent(); cur != nil; cur = cur.Parent() {
			chain = append(chain, cur.Name())
		}
		lines = append(lines, styles.Label.Render("  Parent: ")+styles.Value.Render(strings.Join(chain, " -> ")))
	}

	// Status
	if t.Status() != 0 {
		lines = append(lines, styles.Label.Render("  Status: ")+styles.Value.Render(t.Status().String()))
	}

	// Display hint
	if t.DisplayHint() != "" {
		lines = append(lines, styles.Label.Render("  Hint: ")+styles.Value.Render(t.DisplayHint()))
	}

	// Sizes
	if sizes := t.Sizes(); len(sizes) > 0 {
		var parts []string
		for _, s := range sizes {
			parts = append(parts, s.String())
		}
		lines = append(lines, styles.Label.Render("  Sizes: ")+styles.Value.Render(strings.Join(parts, ", ")))
	}

	// Ranges
	if ranges := t.Ranges(); len(ranges) > 0 {
		var parts []string
		for _, r := range ranges {
			parts = append(parts, r.String())
		}
		lines = append(lines, styles.Label.Render("  Ranges: ")+styles.Value.Render(strings.Join(parts, ", ")))
	}

	// Enums
	if enums := t.Enums(); len(enums) > 0 {
		var parts []string
		for i, e := range enums {
			if i >= 5 {
				parts = append(parts, "...")
				break
			}
			parts = append(parts, fmt.Sprintf("%s(%d)", e.Label, e.Value))
		}
		lines = append(lines, styles.Label.Render("  Values: ")+styles.Value.Render(strings.Join(parts, ", ")))
	}

	// Bits
	if bits := t.Bits(); len(bits) > 0 {
		var parts []string
		for i, bit := range bits {
			if i >= 5 {
				parts = append(parts, "...")
				break
			}
			parts = append(parts, fmt.Sprintf("%s(%d)", bit.Label, bit.Value))
		}
		lines = append(lines, styles.Label.Render("  Bits: ")+styles.Value.Render(strings.Join(parts, ", ")))
	}

	// Description
	if t.Description() != "" {
		desc := truncate(normalizeDescription(t.Description()), tm.width-6)
		lines = append(lines, styles.Label.Render("  ")+styles.Value.Render(desc))
	}

	lines = append(lines, "") // blank separator
	return lines
}
