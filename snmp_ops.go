package main

import (
	"context"
	"errors"

	tea "charm.land/bubbletea/v2"
	"github.com/golangsnmp/gomib/mib"
	"github.com/gosnmp/gosnmp"
)

// doWalk dispatches to Walk or BulkWalk based on the SNMP version.
// SNMPv1 does not support BulkWalk.
func doWalk(client *gosnmp.GoSNMP, oid string, fn gosnmp.WalkFunc) error {
	if client.Version == gosnmp.Version1 {
		return client.Walk(oid, fn)
	}
	return client.BulkWalk(oid, fn)
}

// snmpGetMsg carries the result of an SNMP GET operation.
type snmpGetMsg struct {
	results []gosnmp.SnmpPDU
	err     error
}

// snmpGetNextMsg carries the result of an SNMP GetNext operation.
type snmpGetNextMsg struct {
	oid     string
	results []gosnmp.SnmpPDU
	err     error
}

// opSession tracks a cancellable in-progress SNMP operation (GET, GETNEXT, table walk).
// Only one non-walk SNMP operation runs at a time (enforced by requireConnectedIdle),
// so the model stores a single opSession and calls cancel on disconnect or ESC.
type opSession struct {
	cancel context.CancelFunc
}

// getCmd performs an SNMP GET on the given OIDs.
func getCmd(sess *snmpSession, oids []string) tea.Cmd {
	_, cmd := getCmdWithCancel(sess, oids)
	return cmd
}

// getCmdWithCancel performs an SNMP GET on the given OIDs.
// Returns an opSession whose cancel function aborts the in-flight request,
// and a tea.Cmd that yields snmpGetMsg.
func getCmdWithCancel(sess *snmpSession, oids []string) (*opSession, tea.Cmd) {
	ctx, cancel := context.WithCancel(context.Background())
	op := &opSession{cancel: cancel}

	cmd := func() tea.Msg {
		if sess == nil || !sess.connected || sess.client == nil {
			return snmpGetMsg{err: errors.New("not connected")}
		}

		type getResult struct {
			pkt *gosnmp.SnmpPacket
			err error
		}
		ch := make(chan getResult, 1)
		go func() {
			result, err := sess.client.Get(oids)
			ch <- getResult{pkt: result, err: err}
		}()

		select {
		case <-ctx.Done():
			return snmpGetMsg{err: ctx.Err()}
		case r := <-ch:
			if ctx.Err() != nil {
				return snmpGetMsg{err: ctx.Err()}
			}
			if r.err != nil {
				return snmpGetMsg{err: r.err}
			}
			return snmpGetMsg{results: r.pkt.Variables}
		}
	}

	return op, cmd
}

// getNextCmd performs an SNMP GetNext on the given OID.
func getNextCmd(sess *snmpSession, oid string) tea.Cmd {
	_, cmd := getNextCmdWithCancel(sess, oid)
	return cmd
}

// getNextCmdWithCancel performs an SNMP GetNext on the given OID.
// Returns an opSession whose cancel function aborts the in-flight request,
// and a tea.Cmd that yields snmpGetNextMsg.
func getNextCmdWithCancel(sess *snmpSession, oid string) (*opSession, tea.Cmd) {
	ctx, cancel := context.WithCancel(context.Background())
	op := &opSession{cancel: cancel}

	cmd := func() tea.Msg {
		if sess == nil || !sess.connected || sess.client == nil {
			return snmpGetNextMsg{oid: oid, err: errors.New("not connected")}
		}

		type getResult struct {
			pkt *gosnmp.SnmpPacket
			err error
		}
		ch := make(chan getResult, 1)
		go func() {
			result, err := sess.client.GetNext([]string{oid})
			ch <- getResult{pkt: result, err: err}
		}()

		select {
		case <-ctx.Done():
			return snmpGetNextMsg{oid: oid, err: ctx.Err()}
		case r := <-ch:
			if ctx.Err() != nil {
				return snmpGetNextMsg{oid: oid, err: ctx.Err()}
			}
			if r.err != nil {
				return snmpGetNextMsg{oid: oid, err: r.err}
			}
			return snmpGetNextMsg{oid: oid, results: r.pkt.Variables}
		}
	}

	return op, cmd
}

// walkSession tracks an in-progress SNMP walk.
type walkSession struct {
	ch     <-chan walkBatch
	cancel context.CancelFunc
}

// walkBatch carries a batch of walk results or a completion signal.
type walkBatch struct {
	pdus []gosnmp.SnmpPDU
	done bool
	err  error
}

// snmpWalkBatchMsg carries walk progress to the update loop.
type snmpWalkBatchMsg struct {
	pdus []gosnmp.SnmpPDU
	done bool
	err  error
}

const walkBatchSize = 100

// startWalkCmd begins an SNMP walk and returns the walk session and a command
// that yields the first batch. The walk goroutine sends PDU batches to a channel;
// each handled batch must re-issue waitWalkCmd until done.
func startWalkCmd(sess *snmpSession, rootOID string) (*walkSession, tea.Cmd) {
	ch := make(chan walkBatch, 8)
	ctx, cancel := context.WithCancel(context.Background())
	ws := &walkSession{ch: ch, cancel: cancel}

	go func() {
		var batch []gosnmp.SnmpPDU

		walkFn := func(pdu gosnmp.SnmpPDU) error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			batch = append(batch, pdu)
			if len(batch) >= walkBatchSize {
				b := make([]gosnmp.SnmpPDU, len(batch))
				copy(b, batch)
				select {
				case ch <- walkBatch{pdus: b}:
				case <-ctx.Done():
					return ctx.Err()
				}
				batch = batch[:0]
			}
			return nil
		}

		err := doWalk(sess.client, rootOID, walkFn)

		// Flush remaining results
		if len(batch) > 0 {
			b := make([]gosnmp.SnmpPDU, len(batch))
			copy(b, batch)
			select {
			case ch <- walkBatch{pdus: b}:
			case <-ctx.Done():
			}
		}

		select {
		case ch <- walkBatch{done: true, err: err}:
		case <-ctx.Done():
		}
		close(ch)
	}()

	return ws, waitWalkCmd(ch)
}

// waitWalkCmd returns a command that blocks until the next walk batch is ready.
func waitWalkCmd(ch <-chan walkBatch) tea.Cmd {
	return func() tea.Msg {
		batch, ok := <-ch
		if !ok {
			return snmpWalkBatchMsg{done: true}
		}
		return snmpWalkBatchMsg(batch)
	}
}

// snmpTableDataMsg carries the result of a table data fetch.
type snmpTableDataMsg struct {
	tableName string
	columns   []string   // column names
	rows      [][]string // rows[r][c] = formatted value
	indexCols int        // number of leading index columns
	err       error
}

// tableWalkCmd walks a table OID and organizes the results into rows and columns.
// It uses the MIB to determine column structure and index composition.
func tableWalkCmd(sess *snmpSession, tbl *mib.Object, m *mib.Mib) tea.Cmd {
	_, cmd := tableWalkCmdWithCancel(sess, tbl, m)
	return cmd
}

// tableWalkCmdWithCancel walks a table OID and organizes the results into rows and columns.
// Returns an opSession whose cancel function aborts the in-flight walk,
// and a tea.Cmd that yields snmpTableDataMsg.
func tableWalkCmdWithCancel(sess *snmpSession, tbl *mib.Object, m *mib.Mib) (*opSession, tea.Cmd) {
	ctx, cancel := context.WithCancel(context.Background())
	op := &opSession{cancel: cancel}

	cmd := func() tea.Msg {
		if sess == nil || !sess.connected || sess.client == nil {
			return snmpTableDataMsg{err: errors.New("not connected")}
		}

		tableName := tbl.Name()
		tableOID := tbl.OID().String()

		// Get column definitions
		cols := tbl.Columns()
		if len(cols) == 0 {
			return snmpTableDataMsg{tableName: tableName, err: errors.New("no columns defined")}
		}

		// Build column OID -> index map
		type colInfo struct {
			name string
			obj  *mib.Object
			oid  string
			idx  int // position in output
		}
		colMap := make(map[string]*colInfo, len(cols))
		colList := make([]*colInfo, len(cols))
		for i, col := range cols {
			ci := &colInfo{
				name: col.Name(),
				obj:  col,
				oid:  col.OID().String(),
				idx:  i,
			}
			colMap[ci.oid] = ci
			colList[i] = ci
		}

		// Count index columns
		entry := tbl.Entry()
		indexCols := 0
		if entry != nil {
			iset := indexNameSet(entry.EffectiveIndexes())
			for _, ci := range colList {
				if iset[ci.name] {
					indexCols++
				}
			}
		}

		// Walk the table
		// rowMap: index suffix -> column values
		type rowData struct {
			suffix string
			cells  []string // one per column
		}
		var rowOrder []string
		rowMap := make(map[string]*rowData)

		walkFn := func(pdu gosnmp.SnmpPDU) error {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			// Parse the OID to find which column and index suffix
			oid, err := mib.ParseOID(pdu.Name)
			if err != nil {
				return nil // skip unparseable
			}

			// Find the column this PDU belongs to
			node := m.LongestPrefixByOID(oid)
			if node == nil {
				return nil
			}

			colOID := node.OID().String()
			ci, ok := colMap[colOID]
			if !ok {
				return nil // not one of our columns
			}

			// Index suffix is the part after the column OID
			suffix := oid[len(node.OID()):]
			suffixStr := suffix.String()
			if suffixStr == "" {
				suffixStr = "0"
			}

			rd, exists := rowMap[suffixStr]
			if !exists {
				rd = &rowData{
					suffix: suffixStr,
					cells:  make([]string, len(colList)),
				}
				rowMap[suffixStr] = rd
				rowOrder = append(rowOrder, suffixStr)
			}

			// Format the value
			rd.cells[ci.idx] = formatPDU(pdu, node, m)

			return nil
		}

		err := doWalk(sess.client, tableOID, walkFn)

		if err != nil {
			return snmpTableDataMsg{tableName: tableName, err: err}
		}

		// Build column name list
		colNames := make([]string, len(colList))
		for i, ci := range colList {
			colNames[i] = ci.name
		}

		if len(rowOrder) == 0 {
			return snmpTableDataMsg{tableName: tableName, columns: colNames, rows: nil, indexCols: indexCols}
		}

		// Build output rows in order
		rows := make([][]string, 0, len(rowOrder))
		for _, suffix := range rowOrder {
			rd := rowMap[suffix]
			// Fill empty cells
			for i := range rd.cells {
				if rd.cells[i] == "" {
					rd.cells[i] = "-"
				}
			}
			rows = append(rows, rd.cells)
		}

		return snmpTableDataMsg{
			tableName: tableName,
			columns:   colNames,
			rows:      rows,
			indexCols: indexCols,
		}
	}

	return op, cmd
}
