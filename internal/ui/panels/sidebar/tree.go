// Package sidebar implements the left-hand connection tree panel.
package sidebar

import (
	"github.com/gabrielfloresousion/db-term/internal/types"
	"github.com/gabrielfloresousion/db-term/internal/ui/styles"
)

// NodeKind classifies each node in the connection tree.
type NodeKind int

const (
	NodeConnection NodeKind = iota // top-level named connection
	NodeDatabase                   // database within a connection instance
	NodeSchema                     // schema within a database
	NodeTableGroup                 // "Tables" heading
	NodeViewGroup                  // "Views" heading
	NodeTable                      // individual table
	NodeView                       // individual view
)

// TreeNode is a single node in the connection tree.
type TreeNode struct {
	Kind       NodeKind
	Label      string
	ConnName   string
	Driver     string // database driver for NodeConnection ("postgres"|"mysql"|"sqlserver")
	DBName     string // database name (NodeDatabase and below)
	SchemaName string
	Expanded   bool
	Loading    bool // true while children are being loaded asynchronously
	Children   []*TreeNode
	State      types.ConnState // meaningful only for NodeConnection
}

// IsLeaf reports whether this node can contain children.
func (n *TreeNode) IsLeaf() bool {
	return n.Kind == NodeTable || n.Kind == NodeView
}

// Icon returns the Nerd Font icon for this node kind.
// For NodeConnection, the driver-specific icon is returned.
func (n *TreeNode) Icon() string {
	switch n.Kind {
	case NodeConnection:
		return driverIcon(n.Driver)
	case NodeDatabase:
		return styles.IconSchema
	case NodeSchema:
		return styles.IconSchema
	case NodeTableGroup:
		return styles.IconTable
	case NodeViewGroup:
		return styles.IconView
	case NodeTable:
		return styles.IconTable
	case NodeView:
		return styles.IconView
	}
	return " "
}

// driverIcon returns the engine-specific Nerd Font icon for a driver string.
func driverIcon(driver string) string {
	switch driver {
	case "postgres":
		return styles.IconPostgres
	case "mysql":
		return styles.IconMySQL
	case "sqlserver":
		return styles.IconSQLServer
	default:
		return styles.IconDatabase
	}
}

// ExpandIcon returns the chevron shown before expandable nodes.
func (n *TreeNode) ExpandIcon() string {
	if n.Loading {
		return styles.IconConnecting + " "
	}
	if len(n.Children) == 0 && !n.Loading {
		switch n.Kind {
		case NodeDatabase:
			return styles.IconCollapsed + " "
		}
		return "  "
	}
	if n.Expanded {
		return styles.IconExpanded + " "
	}
	return styles.IconCollapsed + " "
}

// StateIcon returns the connection-state icon (only meaningful for NodeConnection).
func (n *TreeNode) StateIcon() string {
	if n.Kind != NodeConnection {
		return ""
	}
	switch n.State {
	case types.StateConnected:
		return styles.IconConnected
	case types.StateConnecting:
		return styles.IconConnecting
	case types.StateError:
		return styles.IconError
	default:
		return styles.IconDisconnected
	}
}

// BuildConnectionNode creates a top-level connection node populated with
// database children. Each database starts collapsed with no schema children
// — schemas are loaded lazily when the user expands a database node.
func BuildConnectionNode(connName, driver string, state types.ConnState, databases []string) *TreeNode {
	root := &TreeNode{
		Kind:     NodeConnection,
		Label:    connName,
		ConnName: connName,
		Driver:   driver,
		State:    state,
		Expanded: true,
	}
	for _, db := range databases {
		root.Children = append(root.Children, &TreeNode{
			Kind:     NodeDatabase,
			Label:    db,
			ConnName: connName,
			DBName:   db,
		})
	}
	return root
}

// BuildTree constructs the full tree for a single connection from its schemas.
// Used when the connection targets a specific database (no database-level listing).
func BuildTree(connName, driver string, state types.ConnState, schemas []types.Schema) *TreeNode {
	root := &TreeNode{
		Kind:     NodeConnection,
		Label:    connName,
		ConnName: connName,
		Driver:   driver,
		State:    state,
	}

	for _, schema := range schemas {
		schemaNode := buildSchemaNode(connName, "", schema)
		root.Children = append(root.Children, schemaNode)
	}

	return root
}

// AttachSchemas populates the children of a NodeDatabase node with its schemas.
func AttachSchemas(dbNode *TreeNode, schemas []types.Schema) {
	dbNode.Loading = false
	dbNode.Expanded = true
	dbNode.Children = nil
	for _, schema := range schemas {
		schemaNode := buildSchemaNode(dbNode.ConnName, dbNode.DBName, schema)
		dbNode.Children = append(dbNode.Children, schemaNode)
	}
}

func buildSchemaNode(connName, dbName string, schema types.Schema) *TreeNode {
	schemaNode := &TreeNode{
		Kind:       NodeSchema,
		Label:      schema.Name,
		ConnName:   connName,
		DBName:     dbName,
		SchemaName: schema.Name,
	}

	tableGroup := &TreeNode{
		Kind:       NodeTableGroup,
		Label:      "Tables",
		ConnName:   connName,
		DBName:     dbName,
		SchemaName: schema.Name,
	}
	viewGroup := &TreeNode{
		Kind:       NodeViewGroup,
		Label:      "Views",
		ConnName:   connName,
		DBName:     dbName,
		SchemaName: schema.Name,
	}

	for _, tbl := range schema.Tables {
		node := &TreeNode{
			Label:      tbl.Name,
			ConnName:   connName,
			DBName:     dbName,
			SchemaName: schema.Name,
		}
		if tbl.IsView {
			node.Kind = NodeView
			viewGroup.Children = append(viewGroup.Children, node)
		} else {
			node.Kind = NodeTable
			tableGroup.Children = append(tableGroup.Children, node)
		}
	}

	if len(tableGroup.Children) > 0 {
		schemaNode.Children = append(schemaNode.Children, tableGroup)
	}
	if len(viewGroup.Children) > 0 {
		schemaNode.Children = append(schemaNode.Children, viewGroup)
	}
	return schemaNode
}

// FlatList returns a depth-first flattened slice of visible nodes,
// respecting the Expanded flag of each node.
func FlatList(root *TreeNode) []*TreeNode {
	var result []*TreeNode
	appendVisible(root, &result)
	return result
}

func appendVisible(n *TreeNode, out *[]*TreeNode) {
	*out = append(*out, n)
	if n.Expanded {
		for _, child := range n.Children {
			appendVisible(child, out)
		}
	}
}

// FlatItem pairs a node with its computed visual tree-connector prefix.
// Used for rendering only — navigation still uses the plain node slice from
// FlatList so cursor indices stay aligned.
type FlatItem struct {
	Node   *TreeNode
	Prefix string // e.g. "├─ " or "│  └─ "
}

// FlatItems returns a depth-first flattened list of all visible nodes across
// every root, each paired with the tree-connector prefix that expresses its
// position in the hierarchy. Caller iterates both FlatList and FlatItems in
// the same order so that cursor[i] always corresponds to FlatItems[i].
func FlatItems(roots []*TreeNode) []FlatItem {
	var items []FlatItem
	for _, root := range roots {
		items = append(items, FlatItem{Node: root, Prefix: ""})
		if root.Expanded {
			appendItems(root.Children, "  ", &items)
		}
	}
	return items
}

// FlatItemsFiltered is like FlatItems but only includes nodes present in
// visible. Connectors (├─/└─/│) are recomputed based on which siblings are
// visible so the tree looks correct even when many siblings are hidden.
func FlatItemsFiltered(roots []*TreeNode, visible map[*TreeNode]bool) []FlatItem {
	var items []FlatItem
	for _, root := range roots {
		if !visible[root] {
			continue
		}
		items = append(items, FlatItem{Node: root, Prefix: ""})
		appendItemsFiltered(root.Children, "  ", &items, visible)
	}
	return items
}

func appendItems(nodes []*TreeNode, parentPrefix string, items *[]FlatItem) {
	for i, n := range nodes {
		isLast := i == len(nodes)-1
		var connector, childCont string
		if isLast {
			connector = "└─ "
			childCont = "   "
		} else {
			connector = "├─ "
			childCont = "│  "
		}
		*items = append(*items, FlatItem{
			Node:   n,
			Prefix: parentPrefix + connector,
		})
		if n.Expanded {
			appendItems(n.Children, parentPrefix+childCont, items)
		}
	}
}

// appendItemsFiltered walks children recursively, only emitting nodes in
// visible. isLast is computed against visible siblings only so connectors
// are correct (e.g. the last visible child gets └─ even if it has siblings
// that were filtered out).
func appendItemsFiltered(nodes []*TreeNode, parentPrefix string, items *[]FlatItem, visible map[*TreeNode]bool) {
	var vis []*TreeNode
	for _, n := range nodes {
		if visible[n] {
			vis = append(vis, n)
		}
	}
	for i, n := range vis {
		isLast := i == len(vis)-1
		var connector, childCont string
		if isLast {
			connector = "└─ "
			childCont = "   "
		} else {
			connector = "├─ "
			childCont = "│  "
		}
		*items = append(*items, FlatItem{
			Node:   n,
			Prefix: parentPrefix + connector,
		})
		appendItemsFiltered(n.Children, parentPrefix+childCont, items, visible)
	}
}
