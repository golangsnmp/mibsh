package main

import "charm.land/bubbles/v2/key"

type keyMap struct {
	Up           key.Binding
	Down         key.Binding
	PageUp       key.Binding
	PageDown     key.Binding
	Top          key.Binding
	Bottom       key.Binding
	Expand       key.Binding
	Collapse     key.Binding
	ScrollDown   key.Binding
	ScrollUp     key.Binding
	Search       key.Binding
	Filter       key.Binding
	CopyOID      key.Binding
	Back         key.Binding
	Xrefs        key.Binding
	CrossRef     key.Binding
	ChordSNMP    key.Binding
	ChordConnect key.Binding
	ChordView    key.Binding
	Help         key.Binding
	Quit         key.Binding
}

func defaultKeyMap() keyMap {
	return keyMap{
		Up: key.NewBinding(
			key.WithKeys("k", "up"),
			key.WithHelp("k/\u2191", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("j", "down"),
			key.WithHelp("j/\u2193", "down"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("ctrl+u", "pgup"),
			key.WithHelp("ctrl+u", "page up"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("ctrl+d", "pgdown"),
			key.WithHelp("ctrl+d", "page down"),
		),
		Top: key.NewBinding(
			key.WithKeys("home"),
			key.WithHelp("home", "top"),
		),
		Bottom: key.NewBinding(
			key.WithKeys("G", "end"),
			key.WithHelp("G", "bottom"),
		),
		Expand: key.NewBinding(
			key.WithKeys("enter", "l", "right"),
			key.WithHelp("enter/l", "expand"),
		),
		Collapse: key.NewBinding(
			key.WithKeys("h", "left"),
			key.WithHelp("h", "collapse"),
		),
		ScrollDown: key.NewBinding(
			key.WithKeys("J"),
			key.WithHelp("J", "scroll detail down"),
		),
		ScrollUp: key.NewBinding(
			key.WithKeys("K"),
			key.WithHelp("K", "scroll detail up"),
		),
		Search: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "search"),
		),
		Filter: key.NewBinding(
			key.WithKeys("f"),
			key.WithHelp("f", "CEL filter"),
		),
		CopyOID: key.NewBinding(
			key.WithKeys("y"),
			key.WithHelp("y", "copy OID"),
		),
		Back: key.NewBinding(
			key.WithKeys("backspace"),
			key.WithHelp("bksp", "back"),
		),
		Xrefs: key.NewBinding(
			key.WithKeys("x"),
			key.WithHelp("x", "cross-refs"),
		),
		CrossRef: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "jump to node"),
		),
		ChordSNMP: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s ...", "SNMP ops"),
		),
		ChordConnect: key.NewBinding(
			key.WithKeys("c"),
			key.WithHelp("c ...", "connection"),
		),
		ChordView: key.NewBinding(
			key.WithKeys("v"),
			key.WithHelp("v ...", "views"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
	}
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Search, k.Filter, k.Back, k.ChordSNMP, k.ChordConnect, k.Help, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		// Navigation
		{k.Up, k.Down, k.PageUp, k.PageDown, k.Top, k.Bottom, k.Expand, k.Collapse, k.ScrollDown, k.ScrollUp, k.Back},
		// Browsing
		{k.Search, k.Filter, k.CopyOID, k.Xrefs},
		// Chords
		{k.ChordSNMP, k.ChordConnect, k.ChordView},
		// Meta
		{k.CrossRef, k.Help, k.Quit},
	}
}
