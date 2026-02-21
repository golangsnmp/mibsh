package main

import (
	"context"
	"errors"
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/golangsnmp/gomib/mib"
	"github.com/gosnmp/gosnmp"
)

// requireConnectedIdle checks that the SNMP session is connected and no walk is
// in progress. Returns false with an appropriate status message if either check fails.
func (m model) requireConnectedIdle() (tea.Model, tea.Cmd, bool) {
	if !m.snmp.isConnected() {
		ret, cmd := m.setStatusReturn(statusError, "Not connected")
		return ret, cmd, false
	}
	if m.walk != nil {
		ret, cmd := m.setStatusReturn(statusWarn, "Walk in progress")
		return ret, cmd, false
	}
	return m, nil, true
}

// selectedOID holds the node and OID returned by requireSelectedOID.
type selectedOID struct {
	node *mib.Node
	oid  mib.OID
}

// requireSelectedOID returns the selected tree node and its OID, validating that
// the OID is deep enough for SNMP operations. Returns ok=false if the node is nil,
// has no OID, or the OID is too short.
func (m model) requireSelectedOID() (selectedOID, tea.Model, tea.Cmd, bool) {
	node := m.tree.selectedNode()
	if node == nil {
		return selectedOID{}, m, nil, false
	}
	oid := node.OID()
	if oid == nil {
		return selectedOID{}, m, nil, false
	}
	if len(oid) < 2 {
		ret, cmd := m.setStatusReturn(statusWarn, "OID too short for SNMP, select a node deeper in the tree")
		return selectedOID{}, ret, cmd, false
	}
	return selectedOID{node: node, oid: oid}, m, nil, true
}

func (m model) openConnectDialog() (tea.Model, tea.Cmd) {
	var profiles []deviceProfile
	if m.profiles != nil {
		profiles = m.profiles.profiles
	}
	d := newDeviceDialog(m.config, profiles)
	m.dialog = &d
	m.overlay.kind = overlayConnect
	return m, d.focusCmd()
}

// snmpGet issues an SNMP GET for the currently selected tree node.
func (m model) snmpGet() (tea.Model, tea.Cmd) {
	if ret, retCmd, ok := m.requireConnectedIdle(); !ok {
		return ret, retCmd
	}

	sel, ret, retCmd, ok := m.requireSelectedOID()
	if !ok {
		return ret, retCmd
	}

	oidStr := sel.oid.String()
	// Append .0 for scalar nodes
	if sel.node.Kind() == mib.KindScalar {
		oidStr += ".0"
	}

	return m, getCmd(m.snmp, []string{oidStr})
}

// snmpGetNext issues an SNMP GETNEXT for the currently selected tree node.
func (m model) snmpGetNext() (tea.Model, tea.Cmd) {
	if ret, retCmd, ok := m.requireConnectedIdle(); !ok {
		return ret, retCmd
	}

	sel, ret, retCmd, ok := m.requireSelectedOID()
	if !ok {
		return ret, retCmd
	}

	return m, getNextCmd(m.snmp, sel.oid.String())
}

// snmpWalk starts a walk from the currently selected tree node's OID.
func (m model) snmpWalk() (tea.Model, tea.Cmd) {
	if ret, retCmd, ok := m.requireConnectedIdle(); !ok {
		return ret, retCmd
	}

	sel, ret, retCmd, ok := m.requireSelectedOID()
	if !ok {
		return ret, retCmd
	}

	return m.startWalk(sel.oid.String(), "WALK "+sel.node.Name(), sel.oid)
}

// snmpTableData fetches live table data for the currently selected table/row/column node.
func (m model) snmpTableData() (tea.Model, tea.Cmd) {
	if ret, retCmd, ok := m.requireConnectedIdle(); !ok {
		return ret, retCmd
	}

	node := m.tree.selectedNode()
	if node == nil {
		return m, nil
	}

	obj := node.Object()
	if obj == nil {
		return m, nil
	}

	tbl, _ := resolveTable(obj, node.Kind())

	if tbl == nil {
		return m.setStatusReturn(statusWarn, "Not a table node")
	}

	label := "TABLE " + tbl.Name()
	m.tableData.setLoading(label)
	m.bottomPane = bottomTableData
	m.focus = focusResults
	m.updateLayout()

	m.setStatus(statusInfo, "Fetching "+tbl.Name()+"...")

	return m, tableWalkCmd(m.snmp, tbl, m.mib)
}

func (m model) snmpDisconnect() (tea.Model, tea.Cmd) {
	if m.snmp == nil {
		return m, nil
	}
	return m, disconnectCmd(m.snmp)
}

// startQueryWalk starts a walk from a query bar OID (not from tree selection).
func (m model) startQueryWalk(oidStr string) (tea.Model, tea.Cmd) {
	walkOID, _ := mib.ParseOID(oidStr)

	// Try to find a name for the label
	label := "WALK " + oidStr
	if walkOID != nil {
		if node := m.mib.NodeByOID(walkOID); node != nil {
			label = "WALK " + node.Name()
		} else if node := m.mib.LongestPrefixByOID(walkOID); node != nil {
			label = "WALK " + node.Name()
		}
	}

	return m.startWalk(oidStr, label, walkOID)
}

// startWalk begins an SNMP walk, sets up the result group, and switches focus
// to the results pane. Used by both snmpWalk (tree-based) and startQueryWalk
// (query bar-based).
func (m model) startWalk(oidStr, label string, walkOID mib.OID) (tea.Model, tea.Cmd) {
	ws, cmd := startWalkCmd(m.snmp, oidStr)
	m.walk = ws

	g := resultGroup{
		op:          opWalk,
		label:       label,
		walkRootOID: walkOID,
	}
	m.results.addGroup(g)
	m.results.walkStatus = "walking..."
	m.bottomPane = bottomResults
	m.focus = focusResults
	m.updateLayout()

	m.setStatus(statusInfo, label+"...")
	return m, cmd
}

// dispatchQuery executes a parsed query bar command.
func (m model) dispatchQuery(cmd queryCmd) (tea.Model, tea.Cmd) {
	if ret, retCmd, ok := m.requireConnectedIdle(); !ok {
		return ret, retCmd
	}

	if parsed, err := mib.ParseOID(cmd.oid); err == nil && len(parsed) < 2 {
		return m.setStatusReturn(statusWarn, "OID too short for SNMP, use at least 2 arcs (e.g. 1.3)")
	}

	switch cmd.op {
	case queryGet:
		return m, getCmd(m.snmp, []string{cmd.oid})
	case queryGetNext:
		return m, getNextCmd(m.snmp, cmd.oid)
	case queryWalk:
		return m.startQueryWalk(cmd.oid)
	}
	return m, nil
}

// handleSNMPResult creates a result group from SNMP PDUs, formats them, adds
// the group to the results pane, and switches focus to results. Returns the
// group so callers can inspect formatted results for status messages.
func (m *model) handleSNMPResult(op opKind, label string, pdus []gosnmp.SnmpPDU, err error) resultGroup {
	g := resultGroup{op: op, label: label, err: err}
	if err == nil {
		for _, pdu := range pdus {
			g.results = append(g.results, formatPDUToResult(pdu, m.mib))
		}
	}
	m.results.addGroup(g)
	m.bottomPane = bottomResults
	m.focus = focusResults
	m.updateLayout()
	return g
}

func (m model) handleGetResult(msg snmpGetMsg) (tea.Model, tea.Cmd) {
	label := "GET"
	if msg.err == nil && len(msg.results) > 0 {
		label = "GET " + formatPDUToResult(msg.results[0], m.mib).name
	}
	g := m.handleSNMPResult(opGet, label, msg.results, msg.err)

	if msg.err != nil {
		return m.setStatusReturn(statusError, "GET failed: "+msg.err.Error())
	}
	return m.setStatusReturn(statusSuccess, "GET "+g.results[0].name+": ok")
}

func (m model) handleGetNextResult(msg snmpGetNextMsg) (tea.Model, tea.Cmd) {
	m.handleSNMPResult(opGetNext, "GETNEXT "+msg.oid, msg.results, msg.err)

	if msg.err != nil {
		return m.setStatusReturn(statusError, "GETNEXT failed: "+msg.err.Error())
	}
	return m, nil
}

func (m model) handleWalkBatch(msg snmpWalkBatchMsg) (tea.Model, tea.Cmd) {
	// Format and append results
	if len(msg.pdus) > 0 {
		results := make([]snmpResult, 0, len(msg.pdus))
		for _, pdu := range msg.pdus {
			results = append(results, formatPDUToResult(pdu, m.mib))
		}
		m.results.appendResults(results)
	}

	if !msg.done {
		if m.walk != nil {
			return m, waitWalkCmd(m.walk.ch)
		}
		// Walk was cancelled via disconnect, stop processing
		return m, nil
	}

	// Walk complete
	m.results.walkStatus = ""

	if m.walk == nil {
		// Already cleaned up by disconnect
		return m, nil
	}
	m.walk = nil

	g := m.results.history.current()
	count := 0
	if g != nil {
		count = len(g.results)
	}

	if msg.err != nil {
		if errors.Is(msg.err, context.Canceled) {
			m.setStatus(statusInfo, fmt.Sprintf("Walk cancelled (%d results)", count))
		} else {
			if g != nil {
				g.err = msg.err
			}
			m.setStatus(statusError, "Walk failed: "+msg.err.Error())
		}
	} else {
		m.setStatus(statusSuccess, fmt.Sprintf("Walk complete: %d results", count))
	}

	return m, clearStatusAfter(statusDisplayDuration)
}

func (m model) handleTableData(msg snmpTableDataMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.tableData.setError(msg.err)
		return m.setStatusReturn(statusError, "Table fetch failed: "+msg.err.Error())
	}

	m.tableData.setData(msg.tableName, msg.columns, msg.rows, msg.indexCols)
	m.bottomPane = bottomTableData
	m.updateLayout()

	return m.setStatusReturn(statusSuccess, fmt.Sprintf("TABLE %s: %d rows", msg.tableName, len(msg.rows)))
}

func (m model) saveProfile() (tea.Model, tea.Cmd) {
	if m.profiles == nil || !m.snmp.isConnected() {
		return m.setStatusReturn(statusError, "Not connected")
	}

	p := m.lastProfile
	name := p.target
	dp := deviceProfile{
		Name:          name,
		Target:        p.target,
		Community:     p.community,
		Version:       p.version,
		SecurityLevel: p.securityLevel,
		Username:      p.username,
		AuthProto:     p.authProto,
		AuthPass:      p.authPass,
		PrivProto:     p.privProto,
		PrivPass:      p.privPass,
	}
	m.profiles.upsert(dp)
	if err := m.profiles.save(); err != nil {
		return m.setStatusReturn(statusError, "Save failed: "+err.Error())
	}

	return m.setStatusReturn(statusSuccess, "Profile saved: "+name)
}
