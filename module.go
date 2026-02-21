package main

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"github.com/golangsnmp/gomib/mib"
)

// viewLine is a single line in the module/type browser's flattened list.
// Header lines have modIdx >= 0; detail/separator lines have modIdx = -1.
type viewLine struct {
	text   string
	modIdx int // index in filtered, -1 for detail lines
}

// moduleModel is the module browser component.
// It shows a filterable, scrollable list of loaded modules in the detail pane.
type moduleModel struct {
	expandableList
	input    textinput.Model
	all      []*mib.Module
	filtered []*mib.Module
	width    int
	height   int
}

func newModuleModel(m *mib.Mib) moduleModel {
	ti := newStyledInput("filter: ", 128)

	modules := m.Modules()
	slices.SortFunc(modules, func(a, b *mib.Module) int {
		return cmp.Compare(a.Name(), b.Name())
	})

	mm := moduleModel{
		expandableList: newExpandableList(2),
		input:          ti,
		all:            modules,
	}
	mm.applyFilter()
	return mm
}

func (mm *moduleModel) activate() {
	mm.input.SetValue("")
	mm.input.Focus()
	mm.resetExpanded()
	mm.applyFilter()
}

func (mm *moduleModel) deactivate() {
	mm.input.Blur()
}

func (mm *moduleModel) applyFilter() {
	query := strings.ToLower(mm.input.Value())
	mm.filtered = mm.filtered[:0]

	for _, mod := range mm.all {
		if query != "" {
			text := strings.ToLower(mod.Name())
			if !strings.Contains(text, query) {
				continue
			}
		}
		mm.filtered = append(mm.filtered, mod)
	}

	mm.rebuildViewLines(len(mm.filtered),
		func(i int) string { return mm.renderModuleLine(mm.filtered[i]) },
		func(i int) []string { return mm.renderModuleDetail(mm.filtered[i]) },
	)
}

// toggleExpand toggles expand/collapse on the currently selected module.
func (mm *moduleModel) toggleExpand() {
	if mm.expandableList.toggleExpand() {
		mm.rebuildViewLines(len(mm.filtered),
			func(i int) string { return mm.renderModuleLine(mm.filtered[i]) },
			func(i int) []string { return mm.renderModuleDetail(mm.filtered[i]) },
		)
	}
}

func (mm *moduleModel) setSize(width, height int) {
	mm.width = width
	mm.height = height
	mm.lv.SetSize(width, height)
}

func (mm *moduleModel) view() string {

	var b strings.Builder

	// Line 1: filter input
	b.WriteString(mm.input.View())
	b.WriteByte('\n')

	// Line 2: status
	status := fmt.Sprintf("modules: %d/%d  [enter] expand", len(mm.filtered), len(mm.all))
	b.WriteString(styles.StatusText.Render(status))
	b.WriteByte('\n')

	// Module list
	if mm.lv.Len() == 0 {
		b.WriteString(styles.EmptyText.Render("(no modules)"))
		return b.String()
	}

	b.WriteString(mm.lv.Render(renderViewLineFn))

	return b.String()
}

func (mm *moduleModel) renderModuleLine(mod *mib.Module) string {
	lang := mod.Language().String()
	rev := ""
	if revs := mod.Revisions(); len(revs) > 0 {
		rev = revs[0].Date
	}

	var parts []string
	parts = append(parts, styles.Value.Render(mod.Name()))
	parts = append(parts, styles.Label.Render(lang))
	if rev != "" {
		parts = append(parts, styles.Label.Render(rev))
	}
	if path := mod.SourcePath(); path != "" {
		parts = append(parts, styles.Label.Render(path))
	}

	return strings.Join(parts, "  ")
}

func (mm *moduleModel) renderModuleDetail(mod *mib.Module) []string {
	var lines []string

	if mod.SourcePath() != "" {
		lines = append(lines, styles.Label.Render("  Source: ")+styles.Value.Render(mod.SourcePath()))
	}

	if mod.Organization() != "" {
		lines = append(lines, styles.Label.Render("  Org: ")+styles.Value.Render(normalizeDescription(mod.Organization())))
	}

	// Counts
	var counts []string
	if n := len(mod.Objects()); n > 0 {
		counts = append(counts, fmt.Sprintf("%d objects", n))
	}
	if n := len(mod.Types()); n > 0 {
		counts = append(counts, fmt.Sprintf("%d types", n))
	}
	if n := len(mod.Notifications()); n > 0 {
		counts = append(counts, fmt.Sprintf("%d notifications", n))
	}
	if n := len(mod.Groups()); n > 0 {
		counts = append(counts, fmt.Sprintf("%d groups", n))
	}
	if len(counts) > 0 {
		lines = append(lines, styles.Label.Render("  Defines: ")+styles.Value.Render(strings.Join(counts, ", ")))
	}

	// Revisions
	for _, rev := range mod.Revisions() {
		desc := ""
		if rev.Description != "" {
			desc = " - " + truncate(normalizeDescription(rev.Description), 60)
		}
		lines = append(lines, styles.Label.Render("  Rev: ")+styles.Value.Render(rev.Date+desc))
	}

	if mod.Description() != "" {
		desc := truncate(normalizeDescription(mod.Description()), mm.width-6)
		lines = append(lines, styles.Label.Render("  ")+styles.Value.Render(desc))
	}

	lines = append(lines, "") // blank separator
	return lines
}
