package main

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/golangsnmp/gomib/mib"
)

// viewLine is a single line in the module/type browser's flattened list.
// Header lines have itemIdx >= 0; detail/separator lines have itemIdx = -1.
type viewLine struct {
	text    string
	itemIdx int // index in filtered, -1 for detail lines
}

// moduleModel is the module browser component.
// It shows a filterable, scrollable list of loaded modules in the detail pane.
type moduleModel struct {
	expandableList
	all        []*mib.Module
	filtered   []*mib.Module
	importedBy map[string][]string // module name -> sorted list of importing module names
}

func newModuleModel(m *mib.Mib) moduleModel {
	modules := m.Modules()
	slices.SortFunc(modules, func(a, b *mib.Module) int {
		return cmp.Compare(a.Name(), b.Name())
	})

	// Build reverse import index
	importedBy := make(map[string][]string)
	for _, mod := range modules {
		for _, imp := range mod.Imports() {
			importedBy[imp.Module] = append(importedBy[imp.Module], mod.Name())
		}
	}
	for k, v := range importedBy {
		slices.Sort(v)
		importedBy[k] = slices.Compact(v)
	}

	mm := moduleModel{
		expandableList: newExpandableList(2, newStyledInput("filter: ", 128)),
		all:            modules,
		importedBy:     importedBy,
	}
	mm.applyFilter()
	return mm
}

func (mm *moduleModel) activate() {
	mm.expandableList.activate()
	mm.applyFilter()
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

	mm.rebuild()
}

// toggleExpand toggles expand/collapse on the currently selected module.
func (mm *moduleModel) toggleExpand() {
	if mm.expandableList.toggleExpand() {
		mm.rebuild()
	}
}

// rebuild regenerates the flattened view lines from the current filtered list.
func (mm *moduleModel) rebuild() {
	mm.rebuildViewLines(len(mm.filtered),
		func(i int) string { return mm.renderModuleLine(mm.filtered[i]) },
		func(i int) []string { return mm.renderModuleDetail(mm.filtered[i]) },
	)
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

	if mod.ContactInfo() != "" {
		lines = append(lines, styles.Label.Render("  Contact: ")+styles.Value.Render(truncate(normalizeDescription(mod.ContactInfo()), mm.width-12)))
	}

	// Counts
	var counts []string
	if n := len(mod.Tables()); n > 0 {
		counts = append(counts, fmt.Sprintf("%d tables", n))
	}
	if n := len(mod.Scalars()); n > 0 {
		counts = append(counts, fmt.Sprintf("%d scalars", n))
	}
	if n := len(mod.Columns()); n > 0 {
		counts = append(counts, fmt.Sprintf("%d columns", n))
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

	if imports := mod.Imports(); len(imports) > 0 {
		var importParts []string
		for _, imp := range imports {
			importParts = append(importParts, fmt.Sprintf("%s (%d)", imp.Module, len(imp.Symbols)))
		}
		lines = append(lines, styles.Label.Render("  Imports: ")+styles.Value.Render(strings.Join(importParts, ", ")))
	}

	if importers := mm.importedBy[mod.Name()]; len(importers) > 0 {
		label := fmt.Sprintf("%d modules", len(importers))
		if len(importers) <= 5 {
			label = strings.Join(importers, ", ")
		}
		lines = append(lines, styles.Label.Render("  Imported by: ")+styles.Value.Render(label))
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
