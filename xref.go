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
	xrefGroupMember
	xrefNotifObject
	xrefAugments
)

// kindLabel returns a human-readable label for the xref kind.
func (k xrefKind) kindLabel() string {
	switch k {
	case xrefGroup:
		return "group"
	case xrefNotification:
		return "notification"
	case xrefCompliance:
		return "compliance"
	case xrefIndex:
		return "index of"
	case xrefGroupMember:
		return "member"
	case xrefNotifObject:
		return "object"
	case xrefAugments:
		return "augmented by"
	default:
		return "unknown"
	}
}

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

	// Groups reference their members (bidirectional)
	for _, mod := range m.Modules() {
		for _, grp := range mod.Groups() {
			grpName := grp.Node().Name()
			for _, member := range grp.Members() {
				memberName := member.Name()
				if memberName == "" {
					continue
				}
				xm[memberName] = append(xm[memberName], xref{
					kind: xrefGroup,
					name: grpName,
				})
				xm[grpName] = append(xm[grpName], xref{
					kind: xrefGroupMember,
					name: memberName,
				})
			}
		}

		// Notifications reference their objects (bidirectional)
		for _, notif := range mod.Notifications() {
			notifName := notif.Node().Name()
			for _, obj := range notif.Objects() {
				objName := obj.Name()
				if objName == "" {
					continue
				}
				xm[objName] = append(xm[objName], xref{
					kind: xrefNotification,
					name: notifName,
				})
				xm[notifName] = append(xm[notifName], xref{
					kind: xrefNotifObject,
					name: objName,
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

		// Augmentation: row A AUGMENTS row B -> xref on B pointing to A
		for _, obj := range mod.Rows() {
			if target := obj.Augments(); target != nil {
				targetName := target.Name()
				if tbl := target.Table(); tbl != nil {
					targetName = tbl.Name()
				}
				srcName := obj.Name()
				if tbl := obj.Table(); tbl != nil {
					srcName = tbl.Name()
				}
				xm[targetName] = append(xm[targetName], xref{
					kind: xrefAugments,
					name: srcName,
				})
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
		label := ref.kind.kindLabel() + " " + ref.name
		if ref.via != "" {
			label = fmt.Sprintf("%s %s (%s)", ref.kind.kindLabel(), ref.name, ref.via)
		}
		fmt.Fprintf(&b, "    %s\n", label)
	}

	return b.String()
}
