package main

import (
	"fmt"
	"image"
	"time"

	"charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"
	"github.com/golangsnmp/gomib/mib"
)

const doubleClickThreshold = 400 * time.Millisecond

type focus int

const (
	focusTree focus = iota
	focusSearch
	focusFilter
	focusDetail
	focusDiag
	focusModule
	focusTypes
	focusQueryBar
	focusResults
	focusResultFilter
	focusXref
)

// topPane controls which view occupies the top-right sub-pane.
type topPane int

const (
	topDetail topPane = iota
	topDiag
	topTableSchema
	topModule
	topTypes
)

// bottomPane controls which view occupies the bottom-right sub-pane.
// When bottomNone, the top pane takes the full right side.
type bottomPane int

const (
	bottomNone bottomPane = iota
	bottomResults
	bottomTableData
)

// paneID identifies a bordered section for focus highlighting.
type paneID int

const (
	paneTree     paneID = iota
	paneRightTop        // detail, diagnostics, module, table schema
	paneRightBot        // results, table data
)

// appLayout holds computed rectangle regions for each pane.
type appLayout struct {
	area     image.Rectangle // full screen
	header   image.Rectangle // top header bar (1 row)
	tree     image.Rectangle // left pane
	sep      image.Rectangle // vertical border column (1 column)
	rightTop image.Rectangle // top-right sub-pane (detail/diag/table schema/module)
	rightSep image.Rectangle // horizontal border row between top and bottom right
	rightBot image.Rectangle // bottom-right sub-pane (results/table data)
	bottom   image.Rectangle // help/search bar
}

// appConfig holds CLI-provided configuration.
type appConfig struct {
	target    string
	community string
	version   string
}

type model struct {
	mib                *mib.Mib
	tree               treeModel
	detail             detailModel
	search             searchModel
	filterBar          filterBarModel
	diag               diagModel
	tableSchema        tableSchemaModel
	module             moduleModel
	typeBrowser        typeModel
	keys               keyMap
	help               help.Model
	overlay            overlayModel
	status             statusModel
	tooltip            tooltipModel
	topPane            topPane
	bottomPane         bottomPane
	focus              focus
	width              int
	height             int
	treeWidthPct       int
	lastClickRow       int
	lastClickAt        time.Time
	lastResultClickRow int
	lastResultClickAt  time.Time
	hoverRow           int
	stats              string

	// Cross-reference picker
	xrefPicker xrefPickerModel

	// Query bar (Phase 4)
	queryBar queryBarModel

	// SNMP (Phase 2+)
	snmp         *snmpSession
	walk         *walkSession
	results      resultModel
	tableData    tableDataModel
	dialog       *deviceDialogModel
	config       appConfig
	profiles     *profileStore
	lastProfile  snmpProfile // last successful connection, for saving
	pendingChord string      // active chord prefix ("s", "c", "v") or empty
	contextMenu  contextMenuModel
	navStack     []*mib.Node // back-navigation stack (capped at 50)
}

func newApp(m *mib.Mib, cfg appConfig, profiles *profileStore) model {
	tree := newTreeModel(m.Root())
	detail := newDetailModel()
	search := newSearchModel(m)
	filterBar := newFilterBar()
	tree.filter = filterBar.filter
	diag := newDiagModel(m)
	ts := newTableSchemaModel()
	mod := newModuleModel(m)
	typBrowser := newTypeModel(m)
	// Set initial detail to whatever the tree cursor points at
	if node := tree.selectedNode(); node != nil {
		detail.setNode(node)
	}

	h := help.New()
	boldKey := styles.Value.Bold(true)
	h.Styles.ShortKey = boldKey
	h.Styles.ShortDesc = styles.Label
	h.Styles.ShortSeparator = styles.Help.Sep
	h.Styles.FullKey = boldKey
	h.Styles.FullDesc = styles.Label
	h.Styles.FullSeparator = styles.Help.Sep

	stats := fmt.Sprintf("%d modules, %d nodes", len(m.Modules()), m.NodeCount())
	xrefs := buildXrefMap(m)
	detail.xrefs = xrefs

	results := newResultModel()
	results.mib = m

	return model{
		mib:          m,
		tree:         tree,
		detail:       detail,
		search:       search,
		filterBar:    filterBar,
		diag:         diag,
		tableSchema:  ts,
		module:       mod,
		typeBrowser:  typBrowser,
		keys:         defaultKeyMap(),
		help:         h,
		xrefPicker:   newXrefPicker(m),
		queryBar:     newQueryBar(m),
		results:      results,
		tableData:    newTableDataModel(),
		focus:        focusTree,
		hoverRow:     -1,
		stats:        stats,
		config:       cfg,
		profiles:     profiles,
		treeWidthPct: 38,
	}
}

// activePaneID returns the pane that currently has keyboard focus.
func (m model) activePaneID() paneID {
	switch m.focus {
	case focusResults, focusResultFilter:
		return paneRightBot
	case focusDetail, focusDiag, focusModule, focusTypes, focusXref:
		return paneRightTop
	default:
		return paneTree
	}
}

func (m model) Init() tea.Cmd {
	// Auto-connect if target was provided via CLI flags
	if m.config.target != "" {
		return connectCmd(snmpProfile{
			target:    m.config.target,
			community: m.config.community,
			version:   m.config.version,
		})
	}
	return nil
}

const navStackMax = 50

// navPush saves the current tree selection onto the back-navigation stack.
func (m *model) navPush() {
	node := m.tree.selectedNode()
	if node == nil {
		return
	}
	if len(m.navStack) >= navStackMax {
		m.navStack = m.navStack[1:]
	}
	m.navStack = append(m.navStack, node)
}

// navPop pops the most recent node from the back-navigation stack and jumps to it.
// Returns false if the stack was empty.
func (m *model) navPop() bool {
	if len(m.navStack) == 0 {
		return false
	}
	node := m.navStack[len(m.navStack)-1]
	m.navStack = m.navStack[:len(m.navStack)-1]
	m.tree.jumpToNode(node)
	m.syncSelection()
	return true
}
