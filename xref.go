package main

import (
	"fmt"
	"strings"

	"github.com/golangsnmp/gomib/mib"
)

// xrefKind identifies what type of reference this is.
type xrefKind int

const (
	xrefGroup xrefKind = iota
	xrefNotification
	xrefCompliance
	xrefIndex
)

// xref is a single cross-reference from a referencing entity to a referenced object.
type xref struct {
	kind xrefKind
	name string // name of the referencing entity
	via  string // additional context (e.g., module name for compliance)
}

// xrefMap maps node names to their cross-references.
type xrefMap map[string][]xref

func buildXrefMap(m *mib.Mib) xrefMap {
	xm := make(xrefMap)

	// Groups reference their members
	for _, mod := range m.Modules() {
		for _, grp := range mod.Groups() {
			for _, member := range grp.Members() {
				xm[member.Name()] = append(xm[member.Name()], xref{
					kind: xrefGroup,
					name: grp.Node().Name(),
				})
			}
		}

		// Notifications reference their objects
		for _, notif := range mod.Notifications() {
			for _, obj := range notif.Objects() {
				xm[obj.Name()] = append(xm[obj.Name()], xref{
					kind: xrefNotification,
					name: notif.Node().Name(),
				})
			}
		}

		// Compliance modules reference groups and objects
		for _, comp := range mod.Compliances() {
			compName := comp.Name()
			for _, cm := range comp.Modules() {
				for _, g := range cm.MandatoryGroups {
					xm[g] = append(xm[g], xref{
						kind: xrefCompliance,
						name: compName,
						via:  "mandatory",
					})
				}
				for _, cg := range cm.Groups {
					xm[cg.Group] = append(xm[cg.Group], xref{
						kind: xrefCompliance,
						name: compName,
						via:  "group",
					})
				}
				for _, co := range cm.Objects {
					xm[co.Object] = append(xm[co.Object], xref{
						kind: xrefCompliance,
						name: compName,
						via:  "object",
					})
				}
			}
		}

		// Tables reference their columns via index
		for _, tbl := range mod.Tables() {
			entry := tbl.Entry()
			if entry == nil {
				continue
			}
			for _, idx := range entry.EffectiveIndexes() {
				if idx.Object != nil {
					xm[idx.Object.Name()] = append(xm[idx.Object.Name()], xref{
						kind: xrefIndex,
						name: tbl.Name(),
					})
				}
			}
		}
	}

	return xm
}

func renderXrefs(refs []xref) string {
	if len(refs) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteByte('\n')
	b.WriteString(styles.Label.Render("  Referenced by:"))
	b.WriteByte('\n')

	for _, ref := range refs {
		var label string
		switch ref.kind {
		case xrefGroup:
			label = fmt.Sprintf("group %s", ref.name)
		case xrefNotification:
			label = fmt.Sprintf("notification %s", ref.name)
		case xrefCompliance:
			label = fmt.Sprintf("compliance %s (%s)", ref.name, ref.via)
		case xrefIndex:
			label = fmt.Sprintf("index of %s", ref.name)
		}
		fmt.Fprintf(&b, "    %s\n", label)
	}

	return b.String()
}
