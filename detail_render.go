package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/golangsnmp/gomib/mib"
)

// nodeLabel returns the node's name, or "(arc)" as a fallback for unnamed nodes.
func nodeLabel(node *mib.Node) string {
	if name := node.Name(); name != "" {
		return name
	}
	return fmt.Sprintf("(%d)", node.Arc())
}

// writeHeader writes a styled name + underline to b.
func writeHeader(b *strings.Builder, title string) {
	b.WriteString(styles.Header.Info.Render(title))
	b.WriteByte('\n')
	b.WriteString(styles.Header.Underline.Render(strings.Repeat("\u2500", len(title)+4)))
	b.WriteByte('\n')
}

// writeDescription writes a labeled, word-wrapped description block to b.
func writeDescription(b *strings.Builder, desc string, wrapWidth int) {
	b.WriteByte('\n')
	b.WriteString(styles.Label.Render("  Description:"))
	b.WriteByte('\n')
	b.WriteString(wrapText(normalizeDescription(desc), wrapWidth, "    ", "    "))
	b.WriteByte('\n')
}

// formatRangeSuffix formats the first value range as a parenthesized suffix (e.g., " (0..255)").
func formatRangeSuffix(ranges []mib.Range) string {
	if len(ranges) == 0 {
		return ""
	}
	r := ranges[0]
	if r.Min == r.Max {
		return fmt.Sprintf(" (%d)", r.Min)
	}
	return fmt.Sprintf(" (%d..%d)", r.Min, r.Max)
}

// formatSizeSuffix formats the first size constraint as a parenthesized suffix (e.g., " (SIZE(0..255))").
func formatSizeSuffix(sizes []mib.Range) string {
	if len(sizes) == 0 {
		return ""
	}
	s := sizes[0]
	if s.Min == s.Max {
		return fmt.Sprintf(" (SIZE(%d))", s.Min)
	}
	return fmt.Sprintf(" (SIZE(%d..%d))", s.Min, s.Max)
}

// formatRangeList formats all ranges as a pipe-separated list (e.g., "0..100 | 200..300").
func formatRangeList(ranges []mib.Range) string {
	if len(ranges) == 0 {
		return ""
	}
	parts := make([]string, 0, len(ranges))
	for _, r := range ranges {
		if r.Min == r.Max {
			parts = append(parts, fmt.Sprintf("%d", r.Min))
		} else {
			parts = append(parts, fmt.Sprintf("%d..%d", r.Min, r.Max))
		}
	}
	return strings.Join(parts, " | ")
}

// treeIcon returns the expand/collapse/leaf icon for a tree node.
func treeIcon(hasKids, expanded bool) string {
	if hasKids {
		if expanded {
			return ExpandIcon
		}
		return CollapseIcon
	}
	return LeafIcon
}

func (d *detailModel) buildContent() string {
	if d.node == nil {
		return ""
	}
	if d.devMode {
		return d.buildDevContent()
	}

	var b strings.Builder
	node := d.node

	// Header: name with underline
	writeHeader(&b, nodeLabel(node))

	// OID
	oid := node.OID()
	if oid != nil {
		writeLine(&b, "OID", oid.String())
	}

	// Kind
	writeLine(&b, "Kind", node.Kind().String())

	// Module
	if node.Module() != nil {
		writeLine(&b, "Module", node.Module().Name())
		if path := node.Module().SourcePath(); path != "" {
			writeLine(&b, "Source", path)
		}
	}

	// Object details
	if obj := node.Object(); obj != nil {
		d.writeObjectDetails(&b, obj)
	}

	// Notification details
	if notif := node.Notification(); notif != nil {
		d.writeNotificationDetails(&b, notif)
	}

	// Group details
	if grp := node.Group(); grp != nil {
		d.writeGroupDetails(&b, grp)
	}

	// Compliance details
	if comp := node.Compliance(); comp != nil {
		d.writeComplianceDetails(&b, comp)
	}

	// Capability details
	if cap := node.Capability(); cap != nil {
		d.writeCapabilityDetails(&b, cap)
	}

	// Cross-references
	if d.xrefs != nil {
		if refs := d.xrefs[node.Name()]; len(refs) > 0 {
			b.WriteString(renderXrefs(refs))
		}
	}

	return b.String()
}

func (d *detailModel) writeObjectDetails(b *strings.Builder, obj *mib.Object) {
	// Type
	if obj.Type() != nil {
		typ := obj.Type()
		typeName := typ.Name()
		if typeName == "" {
			typeName = typ.Base().String()
		}
		typeDesc := typeName
		if typ.Parent() != nil {
			typeDesc = fmt.Sprintf("%s (%s)", typeName, typ.Base().String())
		}
		typeDesc += formatRangeSuffix(obj.EffectiveRanges())
		typeDesc += formatSizeSuffix(obj.EffectiveSizes())
		writeLine(b, "Type", typeDesc)
		d.writeTypeChain(b, typ)
	} else {
		bits := obj.EffectiveBits()
		enums := obj.EffectiveEnums()
		if len(bits) > 0 {
			writeLine(b, "Type", "BITS")
		} else if len(enums) > 0 {
			writeLine(b, "Type", "INTEGER (enum)")
		}
	}

	// Access
	writeLine(b, "Access", obj.Access().String())

	// Status
	writeLine(b, "Status", obj.Status().String())

	// Index
	if len(obj.Index()) > 0 {
		writeLine(b, "Index", formatIndexList(obj.Index()))
	}

	// Augments
	if obj.Augments() != nil {
		writeLine(b, "Augments", obj.Augments().Name())
	}

	// Units
	if obj.Units() != "" {
		writeLine(b, "Units", obj.Units())
	}

	// DefVal
	dv := obj.DefaultValue()
	if !dv.IsZero() {
		writeLine(b, "DefVal", dv.String())
	}

	// Enums
	enums := obj.EffectiveEnums()
	bits := obj.EffectiveBits()
	if len(enums) > 0 && len(bits) == 0 {
		b.WriteByte('\n')
		b.WriteString(styles.Label.Render("  Values:"))
		b.WriteByte('\n')
		for _, v := range enums {
			fmt.Fprintf(b, "    %s(%d)\n", v.Label, v.Value)
		}
	}

	// Bits
	if len(bits) > 0 {
		b.WriteByte('\n')
		b.WriteString(styles.Label.Render("  Bits:"))
		b.WriteByte('\n')
		for _, bit := range bits {
			fmt.Fprintf(b, "    %s(%d)\n", bit.Label, bit.Value)
		}
	}

	// Description
	if obj.Description() != "" {
		writeDescription(b, obj.Description(), d.width-4)
	}
}

func (d *detailModel) writeNotificationDetails(b *strings.Builder, notif *mib.Notification) {
	writeLine(b, "Status", notif.Status().String())

	if len(notif.Objects()) > 0 {
		b.WriteByte('\n')
		b.WriteString(styles.Label.Render("  Objects:"))
		b.WriteByte('\n')
		for _, obj := range notif.Objects() {
			b.WriteString("    " + obj.Name() + "\n")
		}
	}

	if notif.Description() != "" {
		writeDescription(b, notif.Description(), d.width-4)
	}
}

func (d *detailModel) writeGroupDetails(b *strings.Builder, grp *mib.Group) {
	writeLine(b, "Status", grp.Status().String())

	if len(grp.Members()) > 0 {
		b.WriteByte('\n')
		b.WriteString(styles.Label.Render("  Members:"))
		b.WriteByte('\n')
		for _, member := range grp.Members() {
			name := member.Name()
			if name == "" {
				name = member.OID().String()
			}
			b.WriteString("    " + name + "\n")
		}
	}

	if grp.Description() != "" {
		writeDescription(b, grp.Description(), d.width-4)
	}
}

func (d *detailModel) writeTypeChain(b *strings.Builder, typ *mib.Type) {
	// Build the derivation chain, skipping the immediate type (already shown)
	var chain []*mib.Type
	for cur := typ.Parent(); cur != nil; cur = cur.Parent() {
		chain = append(chain, cur)
	}
	if len(chain) == 0 {
		return
	}

	// Show derivation path
	var parts []string
	parts = append(parts, typeLabel(typ))
	for _, t := range chain {
		parts = append(parts, typeLabel(t))
	}
	writeLine(b, "Chain", strings.Join(parts, " -> "))

	// Show TC details for each textual convention in the chain
	for _, t := range chain {
		if !t.IsTextualConvention() {
			continue
		}
		b.WriteByte('\n')
		tcHeader := t.Name()
		if t.Module() != nil {
			tcHeader += " (" + t.Module().Name() + ")"
		}
		b.WriteString(styles.Label.Render("  TC: "))
		b.WriteString(styles.Value.Render(tcHeader))
		b.WriteByte('\n')

		if t.Status() != 0 {
			b.WriteString("    Status: " + t.Status().String() + "\n")
		}
		if t.DisplayHint() != "" {
			b.WriteString("    Hint: " + t.DisplayHint() + "\n")
		}
		if s := formatRangeList(t.Ranges()); s != "" {
			b.WriteString("    Range: " + s + "\n")
		}
		if s := formatRangeList(t.Sizes()); s != "" {
			b.WriteString("    Size: " + s + "\n")
		}
		if t.Description() != "" {
			desc := normalizeDescription(t.Description())
			b.WriteString(wrapText(desc, d.width-6, "      ", "      "))
			b.WriteByte('\n')
		}
	}
}

func typeLabel(t *mib.Type) string {
	if t.Name() != "" {
		return t.Name()
	}
	return t.Base().String()
}

func (d *detailModel) writeComplianceDetails(b *strings.Builder, comp *mib.Compliance) {
	writeLine(b, "Status", comp.Status().String())

	if comp.Reference() != "" {
		writeLine(b, "Reference", comp.Reference())
	}

	for _, cm := range comp.Modules() {
		b.WriteByte('\n')
		modName := cm.ModuleName
		if modName == "" {
			modName = "(this module)"
		}
		b.WriteString(styles.Label.Render("  MODULE " + modName))
		b.WriteByte('\n')

		if len(cm.MandatoryGroups) > 0 {
			b.WriteString(styles.Label.Render("    Mandatory Groups:"))
			b.WriteByte('\n')
			for _, g := range cm.MandatoryGroups {
				b.WriteString("      " + g + "\n")
			}
		}

		for _, cg := range cm.Groups {
			b.WriteString(styles.Label.Render("    GROUP "))
			b.WriteString(styles.Value.Render(cg.Group))
			b.WriteByte('\n')
			if cg.Description != "" {
				desc := normalizeDescription(cg.Description)
				b.WriteString(wrapText(desc, d.width-8, "        ", "        "))
				b.WriteByte('\n')
			}
		}

		for _, co := range cm.Objects {
			b.WriteString(styles.Label.Render("    OBJECT "))
			b.WriteString(styles.Value.Render(co.Object))
			b.WriteByte('\n')
			if co.MinAccess != nil {
				b.WriteString("      MIN-ACCESS " + co.MinAccess.String() + "\n")
			}
			if co.Description != "" {
				desc := normalizeDescription(co.Description)
				b.WriteString(wrapText(desc, d.width-8, "        ", "        "))
				b.WriteByte('\n')
			}
		}
	}

	if comp.Description() != "" {
		writeDescription(b, comp.Description(), d.width-4)
	}
}

func (d *detailModel) writeCapabilityDetails(b *strings.Builder, cap *mib.Capability) {
	writeLine(b, "Status", cap.Status().String())

	if cap.ProductRelease() != "" {
		writeLine(b, "Release", cap.ProductRelease())
	}

	if cap.Reference() != "" {
		writeLine(b, "Reference", cap.Reference())
	}

	for _, sm := range cap.Supports() {
		b.WriteByte('\n')
		b.WriteString(styles.Label.Render("  SUPPORTS " + sm.ModuleName))
		b.WriteByte('\n')

		if len(sm.Includes) > 0 {
			b.WriteString(styles.Label.Render("    Includes:"))
			b.WriteByte('\n')
			for _, inc := range sm.Includes {
				b.WriteString("      " + inc + "\n")
			}
		}

		for _, ov := range sm.ObjectVariations {
			b.WriteString(styles.Label.Render("    VARIATION "))
			b.WriteString(styles.Value.Render(ov.Object))
			b.WriteByte('\n')
			if ov.Access != nil {
				b.WriteString("      ACCESS " + ov.Access.String() + "\n")
			}
			if ov.Description != "" {
				desc := normalizeDescription(ov.Description)
				b.WriteString(wrapText(desc, d.width-8, "        ", "        "))
				b.WriteByte('\n')
			}
		}

		for _, nv := range sm.NotificationVariations {
			b.WriteString(styles.Label.Render("    VARIATION "))
			b.WriteString(styles.Value.Render(nv.Notification))
			b.WriteByte('\n')
			if nv.Access != nil {
				b.WriteString("      ACCESS " + nv.Access.String() + "\n")
			}
			if nv.Description != "" {
				desc := normalizeDescription(nv.Description)
				b.WriteString(wrapText(desc, d.width-8, "        ", "        "))
				b.WriteByte('\n')
			}
		}
	}

	if cap.Description() != "" {
		writeDescription(b, cap.Description(), d.width-4)
	}
}

func buildBreadcrumb(node *mib.Node) string {
	// Collect ancestors (excluding root which has no meaningful name)
	var ancestors []*mib.Node
	for cur := node.Parent(); cur != nil; cur = cur.Parent() {
		if cur.Name() != "" {
			ancestors = append(ancestors, cur)
		}
	}
	if len(ancestors) == 0 {
		return ""
	}

	// Reverse to go from root toward the node
	slices.Reverse(ancestors)
	var parts []string
	for _, a := range ancestors {
		parts = append(parts, styles.Breadcrumb.Render(a.Name()))
	}

	sep := styles.BreadcrumbSep.Render(" > ")
	return strings.Join(parts, sep)
}

func writeLine(b *strings.Builder, label, value string) {
	b.WriteString(styles.Label.Render(fmt.Sprintf("  %-10s", label)))
	b.WriteString(styles.Value.Render(value))
	b.WriteByte('\n')
}

func normalizeDescription(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	return s
}

// wrapText word-wraps s to fit within width, indenting continuation lines with
// prefix. The first line is indented with firstLinePrefix (pass prefix to get
// uniform indentation, or "" to start the first line without indentation).
func wrapText(s string, width int, prefix string, firstLinePrefix string) string {
	if width <= 0 {
		return firstLinePrefix + s
	}
	var b strings.Builder
	lineWidth := width - len(prefix)
	if lineWidth <= 0 {
		lineWidth = 40
	}
	words := strings.Fields(s)
	b.WriteString(firstLinePrefix)
	lineLen := 0
	for _, word := range words {
		if lineLen > 0 && lineLen+1+len(word) > lineWidth {
			b.WriteByte('\n')
			b.WriteString(prefix)
			lineLen = 0
		}
		if lineLen > 0 {
			b.WriteByte(' ')
			lineLen++
		}
		b.WriteString(word)
		lineLen += len(word)
	}
	return b.String()
}
