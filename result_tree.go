package main

import (
	"slices"
	"strconv"
	"strings"

	"github.com/golangsnmp/gomib/mib"
)

// resultTreeNode represents a node in the hierarchical result display.
// Branch nodes group results by MIB subtree; leaf nodes hold individual results.
type resultTreeNode struct {
	name     string      // display name ("system", "ifDescr", "1")
	oid      mib.OID     // full OID for this node
	mibNode  *mib.Node   // MIB node reference (nil for index-suffix nodes)
	result   *snmpResult // non-nil for leaf nodes with values
	children []*resultTreeNode
	expanded bool
	depth    int
}

// resultTreeRow is a flattened row from the result tree for rendering.
type resultTreeRow struct {
	node    *resultTreeNode
	depth   int
	hasKids bool
}

// buildResultTree constructs a tree from walk results, grouping by MIB hierarchy.
func buildResultTree(results []snmpResult, walkRootOID mib.OID, m *mib.Mib) *resultTreeNode {
	root := &resultTreeNode{
		name:     "results",
		expanded: true,
	}

	for i := range results {
		insertResultIntoTree(root, &results[i], walkRootOID, m)
	}

	return root
}

// insertResultIntoTree adds a single result to the tree, creating intermediate
// group nodes as needed. Preserves expand/collapse state of existing nodes.
func insertResultIntoTree(root *resultTreeNode, res *snmpResult, walkRootOID mib.OID, m *mib.Mib) {
	oid, err := mib.ParseOID(res.oid)
	if err != nil {
		// Can't parse OID, add as a flat child
		leaf := &resultTreeNode{
			name:     res.name,
			oid:      nil,
			result:   res,
			expanded: false,
			depth:    1,
		}
		root.children = append(root.children, leaf)
		return
	}

	node := m.LongestPrefixByOID(oid)
	if node == nil {
		// No MIB match, add as flat child
		leaf := &resultTreeNode{
			name:     res.name,
			oid:      oid,
			result:   res,
			expanded: false,
			depth:    1,
		}
		root.children = append(root.children, leaf)
		return
	}

	// Build the grouping path from the walk root down to this node
	path := buildGroupPath(node, walkRootOID)

	// Navigate/create intermediate group nodes
	current := root
	for _, gn := range path {
		child := findChild(current, gn.name)
		if child == nil {
			child = &resultTreeNode{
				name:     gn.name,
				oid:      gn.oid,
				mibNode:  gn.mibNode,
				expanded: true,
				depth:    current.depth + 1,
			}
			current.children = append(current.children, child)
		}
		current = child
	}

	// Create or find a group node for the MIB node (e.g., "ifDescr"),
	// then attach the result as a leaf (with suffix if present).
	suffix := oid[len(node.OID()):]
	if len(suffix) > 0 {
		// Group by MIB node name, then by suffix
		mibGroup := findChild(current, node.Name())
		if mibGroup == nil {
			mibGroup = &resultTreeNode{
				name:     node.Name(),
				oid:      node.OID(),
				mibNode:  node,
				expanded: true,
				depth:    current.depth + 1,
			}
			current.children = append(current.children, mibGroup)
		}
		suffixStr := formatSuffix(suffix)
		leaf := &resultTreeNode{
			name:     suffixStr,
			oid:      oid,
			result:   res,
			expanded: false,
			depth:    mibGroup.depth + 1,
		}
		mibGroup.children = append(mibGroup.children, leaf)
	} else {
		// No suffix, attach result directly under current group
		leaf := &resultTreeNode{
			name:     node.Name(),
			oid:      oid,
			mibNode:  node,
			result:   res,
			expanded: false,
			depth:    current.depth + 1,
		}
		current.children = append(current.children, leaf)
	}
}

type groupNode struct {
	name    string
	oid     mib.OID
	mibNode *mib.Node
}

// buildGroupPath returns the list of ancestor nodes between walkRoot and the
// result's MIB node, suitable for creating intermediate tree groups.
func buildGroupPath(node *mib.Node, walkRootOID mib.OID) []groupNode {
	// Walk up from node, collecting ancestors. Stop when we reach the walk
	// root or go above it (OID length <= walkRootOID length and is a prefix).
	var ancestors []*mib.Node
	for n := node; n != nil; n = n.Parent() {
		nOID := n.OID()
		if nOID != nil && walkRootOID != nil &&
			len(nOID) <= len(walkRootOID) && walkRootOID.HasPrefix(nOID) {
			break
		}
		ancestors = append(ancestors, n)
	}

	// Reverse to get root-to-leaf order
	slices.Reverse(ancestors)

	// Convert to groupNode slice, skipping the last entry (the result's own
	// MIB node) since it's represented by the leaf/suffix node.
	var path []groupNode
	if len(ancestors) > 1 {
		for _, a := range ancestors[:len(ancestors)-1] {
			path = append(path, groupNode{
				name:    a.Name(),
				oid:     a.OID(),
				mibNode: a,
			})
		}
	}

	return path
}

// findChild finds a child node by name.
func findChild(parent *resultTreeNode, name string) *resultTreeNode {
	for _, c := range parent.children {
		if c.name == name {
			return c
		}
	}
	return nil
}

// formatSuffix formats an OID suffix for display (e.g., ".1.2" -> "1.2").
func formatSuffix(suffix mib.OID) string {
	parts := make([]string, len(suffix))
	for i, arc := range suffix {
		parts[i] = strconv.FormatUint(uint64(arc), 10)
	}
	return strings.Join(parts, ".")
}

// flattenResultTree does a DFS of the result tree, producing rows for rendering.
func flattenResultTree(root *resultTreeNode) []resultTreeRow {
	var rows []resultTreeRow
	for _, child := range root.children {
		flattenResultNode(child, 0, &rows)
	}
	return rows
}

func flattenResultNode(node *resultTreeNode, depth int, rows *[]resultTreeRow) {
	hasKids := len(node.children) > 0
	node.depth = depth
	*rows = append(*rows, resultTreeRow{
		node:    node,
		depth:   depth,
		hasKids: hasKids,
	})
	if node.expanded && hasKids {
		for _, child := range node.children {
			flattenResultNode(child, depth+1, rows)
		}
	}
}

// resultCount returns the total number of leaf results in the subtree.
func (n *resultTreeNode) resultCount() int {
	if n.result != nil {
		return 1
	}
	count := 0
	for _, child := range n.children {
		count += child.resultCount()
	}
	return count
}
