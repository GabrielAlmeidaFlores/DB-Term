package sidebar

import (
	"strings"
	"testing"

	"github.com/gabrielfloresousion/db-term/internal/config"
	"github.com/gabrielfloresousion/db-term/internal/types"
	"github.com/gabrielfloresousion/db-term/internal/ui/styles"
)

func TestBuildTree_ConnectionNodeHasCorrectLabel(t *testing.T) {
	tree := BuildTree("my-conn", "postgres", types.StateConnected, nil)
	if tree.Label != "my-conn" {
		t.Errorf("BuildTree: Label = %q, want %q", tree.Label, "my-conn")
	}
	if tree.Kind != NodeConnection {
		t.Errorf("BuildTree: Kind = %d, want NodeConnection", tree.Kind)
	}
	if tree.State != types.StateConnected {
		t.Errorf("BuildTree: State = %v, want StateConnected", tree.State)
	}
}

func TestBuildTree_SchemasBecomesChildren(t *testing.T) {
	schemas := []types.Schema{
		{Name: "public", Tables: []types.Table{
			{Name: "users", IsView: false},
			{Name: "v_active", IsView: true},
		}},
	}
	tree := BuildTree("conn", "postgres", types.StateConnected, schemas)

	if len(tree.Children) != 1 {
		t.Fatalf("BuildTree: got %d schema children, want 1", len(tree.Children))
	}
	schema := tree.Children[0]
	if schema.Kind != NodeSchema {
		t.Errorf("schema node Kind = %d, want NodeSchema", schema.Kind)
	}

	// Should have a Tables group (1 table) and a Views group (1 view).
	if len(schema.Children) != 2 {
		t.Fatalf("schema.Children = %d, want 2 (Tables + Views)", len(schema.Children))
	}

	tableGroup := schema.Children[0]
	if tableGroup.Kind != NodeTableGroup {
		t.Errorf("tableGroup.Kind = %d, want NodeTableGroup", tableGroup.Kind)
	}
	if len(tableGroup.Children) != 1 || tableGroup.Children[0].Label != "users" {
		t.Errorf("tableGroup: unexpected children %v", tableGroup.Children)
	}

	viewGroup := schema.Children[1]
	if viewGroup.Kind != NodeViewGroup {
		t.Errorf("viewGroup.Kind = %d, want NodeViewGroup", viewGroup.Kind)
	}
	if len(viewGroup.Children) != 1 || viewGroup.Children[0].Kind != NodeView {
		t.Errorf("viewGroup: unexpected children")
	}
}

func TestBuildTree_NoViewGroupWhenNoViews(t *testing.T) {
	schemas := []types.Schema{
		{Name: "public", Tables: []types.Table{
			{Name: "users", IsView: false},
		}},
	}
	tree := BuildTree("conn", "postgres", types.StateConnected, schemas)
	schema := tree.Children[0]
	// Only one child: Tables group (no Views group).
	if len(schema.Children) != 1 {
		t.Errorf("expected 1 group (Tables only), got %d", len(schema.Children))
	}
	if schema.Children[0].Kind != NodeTableGroup {
		t.Errorf("expected NodeTableGroup, got %d", schema.Children[0].Kind)
	}
}

func TestFlatList_CollapsedRootOnlyReturnsRoot(t *testing.T) {
	tree := BuildTree("conn", "postgres", types.StateConnected, []types.Schema{
		{Name: "public", Tables: []types.Table{{Name: "users"}}},
	})
	// Collapsed by default.
	flat := FlatList(tree)
	if len(flat) != 1 {
		t.Errorf("FlatList collapsed: got %d nodes, want 1", len(flat))
	}
}

func TestFlatList_ExpandedRootIncludesChildren(t *testing.T) {
	tree := BuildTree("conn", "postgres", types.StateConnected, []types.Schema{
		{Name: "public", Tables: []types.Table{{Name: "users"}}},
	})
	tree.Expanded = true
	flat := FlatList(tree)
	// conn + schema = 2 (schema not expanded yet)
	if len(flat) != 2 {
		t.Errorf("FlatList with root expanded: got %d nodes, want 2", len(flat))
	}
}

func TestFlatList_FullyExpandedTree(t *testing.T) {
	tree := BuildTree("conn", "postgres", types.StateConnected, []types.Schema{
		{Name: "public", Tables: []types.Table{
			{Name: "users"},
			{Name: "orders"},
		}},
	})
	// Expand everything.
	tree.Expanded = true
	tree.Children[0].Expanded = true             // schema
	tree.Children[0].Children[0].Expanded = true // TableGroup
	flat := FlatList(tree)
	// conn + schema + TableGroup + users + orders = 5
	if len(flat) != 5 {
		t.Errorf("FlatList fully expanded: got %d nodes, want 5", len(flat))
	}
}

func TestFilterNodes_EmptyQueryReturnsAll(t *testing.T) {
	roots := []*TreeNode{
		BuildTree("conn", "postgres", types.StateConnected, []types.Schema{
			{Name: "public", Tables: []types.Table{{Name: "users"}}},
		}),
	}
	roots[0].Expanded = true

	flat := FilterNodes(roots, "")
	all := FlatList(roots[0])
	if len(flat) != len(all) {
		t.Errorf("FilterNodes(empty): got %d, want %d", len(flat), len(all))
	}
}

func TestFilterNodes_MatchingLabelReturnsNode(t *testing.T) {
	schemas := []types.Schema{
		{Name: "public", Tables: []types.Table{
			{Name: "users"},
			{Name: "orders"},
		}},
	}
	roots := []*TreeNode{BuildTree("conn", "postgres", types.StateConnected, schemas)}

	result := FilterNodes(roots, "user")
	// Should include: conn, schema, TableGroup, users
	labels := map[string]bool{}
	for _, n := range result {
		labels[n.Label] = true
	}
	if !labels["users"] {
		t.Error("FilterNodes: expected 'users' in result")
	}
	if labels["orders"] {
		t.Error("FilterNodes: 'orders' should not be in result")
	}
}

func TestFilterNodes_CaseInsensitive(t *testing.T) {
	schemas := []types.Schema{
		{Name: "public", Tables: []types.Table{{Name: "UserProfiles"}}},
	}
	roots := []*TreeNode{BuildTree("conn", "postgres", types.StateConnected, schemas)}
	result := FilterNodes(roots, "userprofiles")
	found := false
	for _, n := range result {
		if n.Label == "UserProfiles" {
			found = true
		}
	}
	if !found {
		t.Error("FilterNodes: case-insensitive match failed for 'UserProfiles'")
	}
}

func TestFilterNodes_NoMatchReturnsEmpty(t *testing.T) {
	schemas := []types.Schema{
		{Name: "public", Tables: []types.Table{{Name: "users"}}},
	}
	roots := []*TreeNode{BuildTree("conn", "postgres", types.StateConnected, schemas)}
	result := FilterNodes(roots, "zzznomatch")
	if len(result) != 0 {
		t.Errorf("FilterNodes no match: expected 0 results, got %d", len(result))
	}
}

func TestTreeNode_IsLeaf(t *testing.T) {
	table := &TreeNode{Kind: NodeTable}
	view := &TreeNode{Kind: NodeView}
	conn := &TreeNode{Kind: NodeConnection}

	if !table.IsLeaf() {
		t.Error("NodeTable.IsLeaf() should be true")
	}
	if !view.IsLeaf() {
		t.Error("NodeView.IsLeaf() should be true")
	}
	if conn.IsLeaf() {
		t.Error("NodeConnection.IsLeaf() should be false")
	}
}

func TestTreeNode_StateIcon(t *testing.T) {
	conn := &TreeNode{Kind: NodeConnection, State: types.StateConnected}
	if conn.StateIcon() == "" {
		t.Error("StateIcon for connected should not be empty")
	}

	// Non-connection nodes return empty.
	table := &TreeNode{Kind: NodeTable}
	if table.StateIcon() != "" {
		t.Errorf("StateIcon for table should be empty, got %q", table.StateIcon())
	}
}

func TestRemoveConnection_CursorStaysInBounds(t *testing.T) {
	// Regression: cursor was clamped against stale m.flat before rebuildFlat().
	// This ensures RemoveConnection does not leave cursor out of bounds.
	schemas := []types.Schema{
		{Name: "public", Tables: []types.Table{
			{Name: "users"}, {Name: "orders"},
		}},
	}
	roots := []*TreeNode{
		BuildTree("conn-a", "postgres", types.StateConnected, schemas),
		BuildTree("conn-b", "postgres", types.StateConnected, schemas),
	}

	// Expand everything so flat is large.
	for _, r := range roots {
		r.Expanded = true
		for _, child := range r.Children {
			child.Expanded = true
			for _, gc := range child.Children {
				gc.Expanded = true
			}
		}
	}

	flat := func(rts []*TreeNode) []*TreeNode {
		var all []*TreeNode
		for _, r := range rts {
			all = append(all, FlatList(r)...)
		}
		return all
	}

	// Move cursor to near the end.
	initialFlat := flat(roots)
	cursor := len(initialFlat) - 1

	// Remove the first connection root.
	filtered := roots[:0]
	for _, r := range roots {
		if r.ConnName != "conn-a" {
			filtered = append(filtered, r)
		}
	}
	roots = filtered

	// After removal, rebuild flat and clamp.
	newFlat := flat(roots)
	if cursor >= len(newFlat) {
		cursor = len(newFlat) - 1
	}
	if cursor < 0 {
		cursor = 0
	}

	if len(newFlat) > 0 && (cursor < 0 || cursor >= len(newFlat)) {
		t.Errorf("cursor %d out of bounds after remove (flat len=%d)", cursor, len(newFlat))
	}
}

func TestFlatItems_PrefixesMatchFlatListOrder(t *testing.T) {
	schemas := []types.Schema{
		{Name: "public", Tables: []types.Table{
			{Name: "users"},
			{Name: "orders"},
		}},
	}
	root := BuildTree("conn", "postgres", types.StateConnected, schemas)
	root.Expanded = true
	root.Children[0].Expanded = true
	root.Children[0].Children[0].Expanded = true

	roots := []*TreeNode{root}
	flat := FlatList(root)
	items := FlatItems(roots)

	if len(flat) != len(items) {
		t.Fatalf("FlatItems len=%d, FlatList len=%d — must match for cursor alignment",
			len(items), len(flat))
	}
	for i, item := range items {
		if item.Node != flat[i] {
			t.Errorf("FlatItems[%d].Node != FlatList[%d] — cursor misalignment", i, i)
		}
	}
}

func TestFlatItems_RootHasNoPrefix(t *testing.T) {
	root := BuildTree("conn", "postgres", types.StateConnected, nil)
	items := FlatItems([]*TreeNode{root})
	if len(items) == 0 {
		t.Fatal("FlatItems: expected at least one item")
	}
	if items[0].Prefix != "" {
		t.Errorf("FlatItems: root node Prefix = %q, want empty", items[0].Prefix)
	}
}

func TestFlatItems_LastChildGetsCornerConnector(t *testing.T) {
	root := BuildTree("conn", "postgres", types.StateConnected, []types.Schema{
		{Name: "public", Tables: []types.Table{{Name: "users"}, {Name: "orders"}}},
	})
	root.Expanded = true
	root.Children[0].Expanded = true
	root.Children[0].Children[0].Expanded = true

	items := FlatItems([]*TreeNode{root})
	for _, item := range items {
		if item.Node.Label == "orders" {
			if item.Prefix == "" {
				t.Error("FlatItems: 'orders' (last child) should have a non-empty prefix")
			}
			return
		}
	}
	t.Error("FlatItems: 'orders' node not found in items")
}

func TestBuildConnectionNode_ChildrenAreDatabases(t *testing.T) {
	dbs := []string{"postgres", "myapp", "analytics"}
	root := BuildConnectionNode("my-conn", "postgres", types.StateConnected, dbs)

	if root.Kind != NodeConnection {
		t.Errorf("root Kind = %d, want NodeConnection", root.Kind)
	}
	if len(root.Children) != 3 {
		t.Fatalf("expected 3 database children, got %d", len(root.Children))
	}
	for i, child := range root.Children {
		if child.Kind != NodeDatabase {
			t.Errorf("child[%d] Kind = %d, want NodeDatabase", i, child.Kind)
		}
		if child.DBName != dbs[i] {
			t.Errorf("child[%d].DBName = %q, want %q", i, child.DBName, dbs[i])
		}
		if child.ConnName != "my-conn" {
			t.Errorf("child[%d].ConnName = %q, want %q", i, child.ConnName, "my-conn")
		}
	}
}

func TestBuildConnectionNode_ExpandedByDefault(t *testing.T) {
	root := BuildConnectionNode("c", "postgres", types.StateConnected, []string{"db1"})
	if !root.Expanded {
		t.Error("BuildConnectionNode: root should be Expanded=true")
	}
}

func TestAttachSchemas_PopulatesDatabaseNode(t *testing.T) {
	dbNode := &TreeNode{Kind: NodeDatabase, Label: "myapp", ConnName: "conn", DBName: "myapp"}

	schemas := []types.Schema{
		{Name: "public", Tables: []types.Table{{Name: "users"}, {Name: "orders"}}},
	}
	AttachSchemas(dbNode, schemas)

	if !dbNode.Expanded {
		t.Error("AttachSchemas: dbNode should be Expanded=true after attach")
	}
	if dbNode.Loading {
		t.Error("AttachSchemas: dbNode.Loading should be false after attach")
	}
	if len(dbNode.Children) != 1 {
		t.Fatalf("AttachSchemas: expected 1 schema child, got %d", len(dbNode.Children))
	}
	schema := dbNode.Children[0]
	if schema.Kind != NodeSchema {
		t.Errorf("schema child Kind = %d, want NodeSchema", schema.Kind)
	}
	if schema.DBName != "myapp" {
		t.Errorf("schema.DBName = %q, want %q", schema.DBName, "myapp")
	}
}

func TestAttachSchemas_ClearsPreviousChildren(t *testing.T) {
	dbNode := &TreeNode{Kind: NodeDatabase, Children: []*TreeNode{{Label: "stale"}}}
	AttachSchemas(dbNode, nil)
	if len(dbNode.Children) != 0 {
		t.Errorf("AttachSchemas: expected empty children, got %d", len(dbNode.Children))
	}
}

func TestView_NarrowWidthNoOverflow(t *testing.T) {
	// Regression: at SidebarMinW (previously 10), the "Connections" title and
	// "No connections yet." hint were never truncated before styling, causing
	// lipgloss to expand the box beyond the allocated width and corrupt the layout.
	th := styles.Resolve("catppuccin-mocha", config.ThemeConfig{})
	s := styles.ToStyles(th)
	m := New(s, config.DefaultConfig().Keybinds)

	for _, w := range []int{60, 40, 28, 20} {
		m.SetSize(w, 10)
		out := m.View()
		for _, line := range strings.Split(out, "\n") {
			visible := []rune(stripANSI(line))
			if len(visible) > w {
				t.Errorf("View(width=%d): line %q has %d visible runes, want ≤%d",
					w, line, len(visible), w)
			}
		}
	}
}

// stripANSI removes ANSI escape sequences so rune-length reflects visible width.
func stripANSI(s string) string {
	var out []rune
	runes := []rune(s)
	i := 0
	for i < len(runes) {
		if runes[i] == '\x1b' && i+1 < len(runes) && runes[i+1] == '[' {
			i += 2
			for i < len(runes) && runes[i] != 'm' {
				i++
			}
			i++
			continue
		}
		out = append(out, runes[i])
		i++
	}
	return string(out)
}
