package main

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/golangsnmp/gomib/mib"
)

// tcBadgeStyle is the pre-built style for [TC] badges in the type browser.
var tcBadgeStyle = lipgloss.NewStyle().Foreground(palette.Green)

// tcFilter controls which types are shown in the type browser.
type tcFilter int

const (
	tcFilterAll    tcFilter = 0 // show all types
	tcFilterTCOnly tcFilter = 1 // show only textual conventions
	tcFilterNonTC  tcFilter = 2 // show only non-TC types
)

// typeModel is the type/TC browser component.
// It shows a filterable, scrollable list of type definitions in the detail pane.
type typeModel struct {
	expandableList
	all      []*mib.Type
	filtered []*mib.Type
	showTC   tcFilter
}

func newTypeModel(m *mib.Mib) typeModel {
	types := m.Types()
	slices.SortFunc(types, func(a, b *mib.Type) int {
		return cmp.Compare(a.Name(), b.Name())
	})

	tm := typeModel{
		expandableList: newExpandableList(2, newStyledInput("filter: ", 128)),
		all:            types,
	}
	tm.applyFilter()
	return tm
}

func (tm *typeModel) activate() {
	tm.expandableList.activate()
	tm.showTC = tcFilterAll
	tm.applyFilter()
}

func (tm *typeModel) applyFilter() {
	query := strings.ToLower(tm.input.Value())
	tm.filtered = tm.filtered[:0]

	for _, t := range tm.all {
		// TC filter
		switch tm.showTC {
		case tcFilterTCOnly:
			if !t.IsTextualConvention() {
				continue
			}
		case tcFilterNonTC:
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

	tm.rebuild()
}

// toggleExpand toggles expand/collapse on the currently selected type.
func (tm *typeModel) toggleExpand() {
	if tm.expandableList.toggleExpand() {
		tm.rebuild()
	}
}

// rebuild regenerates the flattened view lines from the current filtered list.
func (tm *typeModel) rebuild() {
	tm.rebuildViewLines(len(tm.filtered),
		func(i int) string { return tm.renderTypeLine(tm.filtered[i]) },
		func(i int) []string { return tm.renderTypeDetail(tm.filtered[i]) },
	)
}

// cycleTCFilter cycles the TC filter mode: all -> TC only -> non-TC -> all.
func (tm *typeModel) cycleTCFilter() {
	tm.showTC = (tm.showTC + 1) % (tcFilterNonTC + 1)
	tm.resetExpanded()
	tm.applyFilter()
}

func (tm *typeModel) view() string {
	var b strings.Builder

	// Line 1: filter input
	b.WriteString(tm.input.View())
	b.WriteByte('\n')

	// Line 2: status
	tcLabel := "all"
	switch tm.showTC {
	case tcFilterTCOnly:
		tcLabel = "TC only"
	case tcFilterNonTC:
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
	var parts []string
	parts = append(parts, styles.Value.Render(t.Name()))
	parts = append(parts, styles.Label.Render(t.EffectiveBase().String()))
	if t.IsTextualConvention() {
		parts = append(parts, tcBadgeStyle.Render("[TC]"))
	}
	if t.IsCounter() {
		parts = append(parts, tcBadgeStyle.Render("[Ctr]"))
	} else if t.IsGauge() {
		parts = append(parts, tcBadgeStyle.Render("[Gauge]"))
	} else if t.IsBits() {
		parts = append(parts, tcBadgeStyle.Render("[Bits]"))
	} else if t.IsEnumeration() {
		parts = append(parts, tcBadgeStyle.Render("[Enum]"))
	} else if t.IsString() {
		parts = append(parts, tcBadgeStyle.Render("[Str]"))
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

	// Reference
	if t.Reference() != "" {
		lines = append(lines, styles.Label.Render("  Ref: ")+styles.Value.Render(normalizeDescription(t.Reference())))
	}

	// Display hint
	if t.DisplayHint() != "" {
		hint := t.DisplayHint()
		if eh := t.EffectiveDisplayHint(); eh != "" && eh != hint {
			hint += styles.Label.Render("  effective: ") + styles.Value.Render(eh)
		}
		lines = append(lines, styles.Label.Render("  Hint: ")+styles.Value.Render(hint))
	} else if eh := t.EffectiveDisplayHint(); eh != "" {
		lines = append(lines, styles.Label.Render("  Hint: ")+styles.Subtle.Render("(inherited) ")+styles.Value.Render(eh))
	}

	// Sizes
	if sizes := t.Sizes(); len(sizes) > 0 {
		lines = append(lines, styles.Label.Render("  Sizes: ")+styles.Value.Render(formatRangeList(sizes)))
	} else if es := t.EffectiveSizes(); len(es) > 0 {
		lines = append(lines, styles.Label.Render("  Sizes: ")+styles.Subtle.Render("(inherited) ")+styles.Value.Render(formatRangeList(es)))
	}

	// Ranges
	if ranges := t.Ranges(); len(ranges) > 0 {
		lines = append(lines, styles.Label.Render("  Ranges: ")+styles.Value.Render(formatRangeList(ranges)))
	} else if er := t.EffectiveRanges(); len(er) > 0 {
		lines = append(lines, styles.Label.Render("  Ranges: ")+styles.Subtle.Render("(inherited) ")+styles.Value.Render(formatRangeList(er)))
	}

	// Enums
	if enums := t.Enums(); len(enums) > 0 {
		lines = append(lines, styles.Label.Render("  Values: ")+styles.Value.Render(formatNamedValues(enums)))
	} else if ee := t.EffectiveEnums(); len(ee) > 0 {
		lines = append(lines, styles.Label.Render("  Values: ")+styles.Subtle.Render("(inherited) ")+styles.Value.Render(formatNamedValues(ee)))
	}

	// Bits
	if bits := t.Bits(); len(bits) > 0 {
		lines = append(lines, styles.Label.Render("  Bits: ")+styles.Value.Render(formatNamedValues(bits)))
	} else if eb := t.EffectiveBits(); len(eb) > 0 {
		lines = append(lines, styles.Label.Render("  Bits: ")+styles.Subtle.Render("(inherited) ")+styles.Value.Render(formatNamedValues(eb)))
	}

	// Description
	if t.Description() != "" {
		desc := truncate(normalizeDescription(t.Description()), tm.width-6)
		lines = append(lines, styles.Label.Render("  ")+styles.Value.Render(desc))
	}

	lines = append(lines, "") // blank separator
	return lines
}

// formatNamedValues formats up to 5 named values as "label(value), ..." with ellipsis if truncated.
func formatNamedValues(values []mib.NamedValue) string {
	var parts []string
	for i, v := range values {
		if i >= 5 {
			parts = append(parts, "...")
			break
		}
		parts = append(parts, fmt.Sprintf("%s(%d)", v.Label, v.Value))
	}
	return strings.Join(parts, ", ")
}
