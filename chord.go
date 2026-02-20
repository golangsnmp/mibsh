package main

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// chordAction is a single sub-key action within a chord group.
type chordAction struct {
	key   string
	label string
}

// chordGroup is a prefix key with its available sub-actions.
type chordGroup struct {
	prefix  string
	label   string
	actions []chordAction
}

// chordGroups returns the static chord group definitions.
func chordGroups() []chordGroup {
	return []chordGroup{
		{
			prefix: "s",
			label:  "SNMP",
			actions: []chordAction{
				{key: "g", label: "GET"},
				{key: "n", label: "GETNEXT"},
				{key: "w", label: "WALK"},
				{key: "t", label: "TABLE"},
				{key: "q", label: "query by OID"},
			},
		},
		{
			prefix: "c",
			label:  "Connection",
			actions: []chordAction{
				{key: "c", label: "connect"},
				{key: "d", label: "disconnect"},
				{key: "s", label: "save profile"},
			},
		},
		{
			prefix: "v",
			label:  "View",
			actions: []chordAction{
				{key: "d", label: "diagnostics"},
				{key: "m", label: "modules"},
				{key: "y", label: "types"},
				{key: "s", label: "table schema"},
				{key: "r", label: "results"},
				{key: "i", label: "dev info"},
				{key: "t", label: "tree/flat (results)"},
				{key: "o", label: "raw OIDs (results)"},
				{key: ",", label: "shrink tree"},
				{key: ".", label: "grow tree"},
			},
		},
	}
}

// renderChordHint builds the popup content for a chord group.
func renderChordHint(group chordGroup) string {
	ks := styles.Value.Bold(true)
	ds := styles.Label

	var b strings.Builder
	b.WriteString(styles.Dialog.Title.Render(group.label))
	b.WriteString("\n")

	for _, a := range group.actions {
		b.WriteString("\n")
		b.WriteString("  ")
		b.WriteString(ks.Render(a.key))
		b.WriteString("  ")
		b.WriteString(ds.Render(a.label))
	}

	content := b.String()
	bg := lipgloss.Color("#2D2C35")
	return padContentBg(content, bg)
}
