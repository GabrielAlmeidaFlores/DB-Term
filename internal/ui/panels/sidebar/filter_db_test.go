package sidebar

import (
	"strings"
	"testing"

	"github.com/gabrielfloresousion/db-term/internal/types"
)

func TestFilterNodes_WithDatabaseLevel_Loaded(t *testing.T) {
	root := BuildConnectionNode("conn", "postgres", types.StateConnected, []string{"demo", "other"})
	AttachSchemas(root.Children[0], []types.Schema{
		{Name: "public", Tables: []types.Table{
			{Name: "rastro_atividade"},
			{Name: "users"},
		}},
	})

	result := FilterNodes([]*TreeNode{root}, "rastro")
	found := false
	for _, n := range result {
		if n.Label == "rastro_atividade" {
			found = true
		}
	}
	if !found {
		t.Errorf("filter should find 'rastro_atividade' under NodeDatabase; got %d nodes: %v",
			len(result), labelsOf(result))
	}
}

func TestFilterNodes_WithDatabaseLevel_NotLoaded(t *testing.T) {
	root := BuildConnectionNode("conn", "postgres", types.StateConnected, []string{"demo"})

	result := FilterNodes([]*TreeNode{root}, "rastro")
	for _, n := range result {
		if n.Label == "rastro_atividade" {
			t.Error("should not find table that was never loaded")
		}
	}
}

func TestFlatItemsFiltered_LastVisibleSiblingGetsCorner(t *testing.T) {
	// Regression: when siblings are filtered out, the last *visible* sibling
	// must use └─ (corner), not ├─ (tee). Previously all filtered nodes used
	// empty prefixes because FlatItems was bypassed entirely.
	schemas := []types.Schema{
		{Name: "public", Tables: []types.Table{
			{Name: "alpha"},
			{Name: "rastro_atividade"},
			{Name: "zeta"},
		}},
	}
	root := BuildTree("conn", "postgres", types.StateConnected, schemas)
	root.Expanded = true
	// Expand schema and table-group so their children are reachable
	for _, child := range root.Children {
		child.Expanded = true
		for _, gc := range child.Children {
			gc.Expanded = true
		}
	}

	filtered := FilterNodes([]*TreeNode{root}, "rastro")
	visible := make(map[*TreeNode]bool, len(filtered))
	for _, n := range filtered {
		visible[n] = true
	}

	items := FlatItemsFiltered([]*TreeNode{root}, visible)
	for _, item := range items {
		if item.Node.Label == "rastro_atividade" {
			if !strings.HasSuffix(item.Prefix, "└─ ") {
				t.Errorf("last visible sibling should have corner connector '└─ '; got prefix %q", item.Prefix)
			}
			return
		}
	}
	t.Error("rastro_atividade not found in FlatItemsFiltered result")
}

func labelsOf(nodes []*TreeNode) []string {
	s := make([]string, len(nodes))
	for i, n := range nodes {
		s[i] = n.Label
	}
	return s
}
