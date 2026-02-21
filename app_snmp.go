package main

import (
	"context"
	"errors"
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/golangsnmp/gomib/mib"
	"github.com/golangsnmp/mibsh/internal/profile"
	"github.com/golangsnmp/mibsh/internal/snmp"
	"github.com/gosnmp/gosnmp"
)

// requireConnectedIdle checks that the SNMP session is connected and no walk is
// in progress. Returns false with an appropriate status message if either check fails.
func (m model) requireConnectedIdle() (tea.Model, tea.Cmd, bool) {
	if !m.snmp.IsConnected() {
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
	var profiles []profile.Device
	if m.profiles != nil {
		profiles = m.profiles.Devices()
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

	return m, snmp.GetCmd(m.snmp, []string{oidStr})
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

	return m, snmp.GetNextCmd(m.snmp, sel.oid.String())
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

	return m, snmp.TableWalkCmd(m.snmp, tbl, m.mib)
}

func (m model) snmpDisconnect() (tea.Model, tea.Cmd) {
	if m.snmp == nil {
		return m, nil
	}
	return m, snmp.DisconnectCmd(m.snmp)
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
	ws, cmd := snmp.StartWalkCmd(m.snmp, oidStr)
	m.walk = ws

	g := snmp.ResultGroup{
		Op:          snmp.OpWalk,
		Label:       label,
		WalkRootOID: walkOID,
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
		return m, snmp.GetCmd(m.snmp, []string{cmd.oid})
	case queryGetNext:
		return m, snmp.GetNextCmd(m.snmp, cmd.oid)
	case queryWalk:
		return m.startQueryWalk(cmd.oid)
	}
	return m, nil
}

// handleSNMPResult creates a result group from SNMP PDUs, formats them, adds
// the group to the results pane, and switches focus to results. Returns the
// group so callers can inspect formatted results for status messages.
func (m *model) handleSNMPResult(op snmp.OpKind, label string, pdus []gosnmp.SnmpPDU, err error) snmp.ResultGroup {
	g := snmp.ResultGroup{Op: op, Label: label, Err: err}
	if err == nil {
		for _, pdu := range pdus {
			g.Results = append(g.Results, snmp.FormatPDUToResult(pdu, m.mib))
		}
	}
	m.results.addGroup(g)
	m.bottomPane = bottomResults
	m.focus = focusResults
	m.updateLayout()
	return g
}

func (m model) handleGetResult(msg snmp.GetMsg) (tea.Model, tea.Cmd) {
	label := "GET"
	if msg.Err == nil && len(msg.Results) > 0 {
		label = "GET " + snmp.FormatPDUToResult(msg.Results[0], m.mib).Name
	}
	g := m.handleSNMPResult(snmp.OpGet, label, msg.Results, msg.Err)

	if msg.Err != nil {
		return m.setStatusReturn(statusError, "GET failed: "+msg.Err.Error())
	}
	return m.setStatusReturn(statusSuccess, "GET "+g.Results[0].Name+": ok")
}

func (m model) handleGetNextResult(msg snmp.GetNextMsg) (tea.Model, tea.Cmd) {
	m.handleSNMPResult(snmp.OpGetNext, "GETNEXT "+msg.OID, msg.Results, msg.Err)

	if msg.Err != nil {
		return m.setStatusReturn(statusError, "GETNEXT failed: "+msg.Err.Error())
	}
	return m, nil
}

func (m model) handleWalkBatch(msg snmp.WalkBatchMsg) (tea.Model, tea.Cmd) {
	// Format and append results
	if len(msg.PDUs) > 0 {
		results := make([]snmp.Result, 0, len(msg.PDUs))
		for _, pdu := range msg.PDUs {
			results = append(results, snmp.FormatPDUToResult(pdu, m.mib))
		}
		m.results.appendResults(results)
	}

	if !msg.Done {
		if m.walk != nil {
			return m, snmp.WaitWalkCmd(m.walk.Ch)
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

	g := m.results.history.Current()
	count := 0
	if g != nil {
		count = len(g.Results)
	}

	if msg.Err != nil {
		if errors.Is(msg.Err, context.Canceled) {
			m.setStatus(statusInfo, fmt.Sprintf("Walk cancelled (%d results)", count))
		} else {
			if g != nil {
				g.Err = msg.Err
			}
			m.setStatus(statusError, "Walk failed: "+msg.Err.Error())
		}
	} else {
		m.setStatus(statusSuccess, fmt.Sprintf("Walk complete: %d results", count))
	}

	return m, clearStatusAfter(statusDisplayDuration)
}

func (m model) handleTableData(msg snmp.TableDataMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.tableData.setError(msg.Err)
		return m.setStatusReturn(statusError, "Table fetch failed: "+msg.Err.Error())
	}

	m.tableData.setData(msg.TableName, msg.Columns, msg.Rows, msg.IndexCols)
	m.bottomPane = bottomTableData
	m.updateLayout()

	return m.setStatusReturn(statusSuccess, fmt.Sprintf("TABLE %s: %d rows", msg.TableName, len(msg.Rows)))
}

func (m model) saveProfile() (tea.Model, tea.Cmd) {
	if m.profiles == nil || !m.snmp.IsConnected() {
		return m.setStatusReturn(statusError, "Not connected")
	}

	m.profiles.Upsert(m.lastDevice)
	if err := m.profiles.Save(); err != nil {
		return m.setStatusReturn(statusError, "Save failed: "+err.Error())
	}

	return m.setStatusReturn(statusSuccess, "Profile saved: "+m.lastDevice.Name)
}
