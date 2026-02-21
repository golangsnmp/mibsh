package main

import (
	"context"
	"errors"
	"slices"

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

// snmpOpFunc performs an SNMP operation on a connected client, returning a packet.
type snmpOpFunc func(client *gosnmp.GoSNMP) (*gosnmp.SnmpPacket, error)

// snmpResultFunc builds a tea.Msg from PDU results or an error.
type snmpResultFunc func(results []gosnmp.SnmpPDU, err error) tea.Msg

// snmpCmdWithCancel runs an SNMP operation in a cancellable goroutine.
// It checks that the session is connected, launches the operation, and wraps
// the result using the provided buildMsg function.
func snmpCmdWithCancel(
	sess *snmpSession,
	op snmpOpFunc,
	buildMsg snmpResultFunc,
) tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	_ = cancel // cancellation support reserved for future use

	cmd := func() tea.Msg {
		if !sess.isConnected() {
			return buildMsg(nil, errors.New("not connected"))
		}

		type opResult struct {
			pkt *gosnmp.SnmpPacket
			err error
		}
		ch := make(chan opResult, 1)
		go func() {
			pkt, err := op(sess.client)
			ch <- opResult{pkt: pkt, err: err}
		}()

		select {
		case <-ctx.Done():
			return buildMsg(nil, ctx.Err())
		case r := <-ch:
			if ctx.Err() != nil {
				return buildMsg(nil, ctx.Err())
			}
			if r.err != nil {
				return buildMsg(nil, r.err)
			}
			return buildMsg(r.pkt.Variables, nil)
		}
	}

	return cmd
}

// getCmd performs an SNMP GET on the given OIDs.
func getCmd(sess *snmpSession, oids []string) tea.Cmd {
	return snmpCmdWithCancel(sess,
		func(client *gosnmp.GoSNMP) (*gosnmp.SnmpPacket, error) {
			return client.Get(oids)
		},
		func(results []gosnmp.SnmpPDU, err error) tea.Msg {
			return snmpGetMsg{results: results, err: err}
		},
	)
}

// getNextCmd performs an SNMP GetNext on the given OID.
func getNextCmd(sess *snmpSession, oid string) tea.Cmd {
	return snmpCmdWithCancel(sess,
		func(client *gosnmp.GoSNMP) (*gosnmp.SnmpPacket, error) {
			return client.GetNext([]string{oid})
		},
		func(results []gosnmp.SnmpPDU, err error) tea.Msg {
			return snmpGetNextMsg{oid: oid, results: results, err: err}
		},
	)
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
				b := slices.Clone(batch)
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
			b := slices.Clone(batch)
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

// tableColInfo describes a single column in a table walk.
type tableColInfo struct {
	name string
	oid  string
	idx  int // position in output
}

// tableRowData holds the cell values for one row, keyed by index suffix.
type tableRowData struct {
	suffix string
	cells  []string // one per column
}

// tableSchema holds the column layout and index count for a table.
type tableSchema struct {
	colMap   map[string]*tableColInfo // column OID -> info
	colList  []*tableColInfo          // ordered columns
	colNames []string                 // column names in order
	indexCols int                     // number of leading index columns
}

// buildTableSchema extracts column definitions and index count from a table object.
func buildTableSchema(tbl *mib.Object) *tableSchema {
	cols := tbl.Columns()
	colMap := make(map[string]*tableColInfo, len(cols))
	colList := make([]*tableColInfo, len(cols))
	colNames := make([]string, len(cols))

	for i, col := range cols {
		ci := &tableColInfo{
			name: col.Name(),
			oid:  col.OID().String(),
			idx:  i,
		}
		colMap[ci.oid] = ci
		colList[i] = ci
		colNames[i] = ci.name
	}

	indexCols := 0
	if entry := tbl.Entry(); entry != nil {
		iset := indexNameSet(entry.EffectiveIndexes())
		for _, ci := range colList {
			if iset[ci.name] {
				indexCols++
			}
		}
	}

	return &tableSchema{
		colMap:    colMap,
		colList:   colList,
		colNames:  colNames,
		indexCols: indexCols,
	}
}

// tableWalkCollector accumulates PDU data during a table walk, organizing
// values into rows keyed by index suffix.
type tableWalkCollector struct {
	schema   *tableSchema
	m        *mib.Mib
	rowMap   map[string]*tableRowData
	rowOrder []string
}

// newTableWalkCollector creates a collector for the given schema and MIB.
func newTableWalkCollector(schema *tableSchema, m *mib.Mib) *tableWalkCollector {
	return &tableWalkCollector{
		schema: schema,
		m:      m,
		rowMap: make(map[string]*tableRowData),
	}
}

// handlePDU processes a single walk PDU, placing the formatted value into
// the correct row and column. It returns nil for PDUs that cannot be mapped.
func (c *tableWalkCollector) handlePDU(pdu gosnmp.SnmpPDU) error {
	oid, err := mib.ParseOID(pdu.Name)
	if err != nil {
		return nil // skip unparseable
	}

	node := c.m.LongestPrefixByOID(oid)
	if node == nil {
		return nil
	}

	colOID := node.OID().String()
	ci, ok := c.schema.colMap[colOID]
	if !ok {
		return nil // not one of our columns
	}

	suffix := oid[len(node.OID()):]
	suffixStr := suffix.String()
	if suffixStr == "" {
		suffixStr = "0"
	}

	rd, exists := c.rowMap[suffixStr]
	if !exists {
		rd = &tableRowData{
			suffix: suffixStr,
			cells:  make([]string, len(c.schema.colList)),
		}
		c.rowMap[suffixStr] = rd
		c.rowOrder = append(c.rowOrder, suffixStr)
	}

	rd.cells[ci.idx] = formatPDU(pdu, node, c.m)
	return nil
}

// buildTableRows converts collected walk data into ordered row slices,
// filling empty cells with "-".
func (c *tableWalkCollector) buildTableRows() [][]string {
	if len(c.rowOrder) == 0 {
		return nil
	}

	rows := make([][]string, 0, len(c.rowOrder))
	for _, suffix := range c.rowOrder {
		rd := c.rowMap[suffix]
		for i := range rd.cells {
			if rd.cells[i] == "" {
				rd.cells[i] = "-"
			}
		}
		rows = append(rows, rd.cells)
	}
	return rows
}

// tableWalkCmd walks a table OID and organizes the results into rows and columns.
// It uses the MIB to determine column structure and index composition.
func tableWalkCmd(sess *snmpSession, tbl *mib.Object, m *mib.Mib) tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	_ = cancel // cancellation support reserved for future use

	return func() tea.Msg {
		if !sess.isConnected() {
			return snmpTableDataMsg{err: errors.New("not connected")}
		}

		tableName := tbl.Name()
		tableOID := tbl.OID().String()

		cols := tbl.Columns()
		if len(cols) == 0 {
			return snmpTableDataMsg{tableName: tableName, err: errors.New("no columns defined")}
		}

		schema := buildTableSchema(tbl)
		collector := newTableWalkCollector(schema, m)

		walkFn := func(pdu gosnmp.SnmpPDU) error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return collector.handlePDU(pdu)
		}

		if err := doWalk(sess.client, tableOID, walkFn); err != nil {
			return snmpTableDataMsg{tableName: tableName, err: err}
		}

		return snmpTableDataMsg{
			tableName: tableName,
			columns:   schema.colNames,
			rows:      collector.buildTableRows(),
			indexCols: schema.indexCols,
		}
	}
}
