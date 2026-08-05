package sidebar

import "strings"

// FilterNodes returns a flattened list of visible nodes whose Label contains
// query (case-insensitive). Parent nodes (connection, schema, group) are always
// included when they have at least one matching descendant.
// When query is empty, all nodes from all roots are returned unchanged.
func FilterNodes(roots []*TreeNode, query string) []*TreeNode {
	if query == "" {
		var all []*TreeNode
		for _, r := range roots {
			all = append(all, FlatList(r)...)
		}
		return all
	}

	q := strings.ToLower(query)
	var result []*TreeNode
	for _, r := range roots {
		collectMatching(r, q, &result)
	}
	return result
}

// collectMatching appends node (and its matching descendants) to out if the
// node itself matches or has at least one matching descendant.
func collectMatching(n *TreeNode, q string, out *[]*TreeNode) bool {
	if n.IsLeaf() {
		if strings.Contains(strings.ToLower(n.Label), q) {
			*out = append(*out, n)
			return true
		}
		return false
	}

	startLen := len(*out)
	*out = append(*out, n)
	childMatch := false
	for _, child := range n.Children {
		if collectMatching(child, q, out) {
			childMatch = true
		}
	}
	if !childMatch {
		*out = (*out)[:startLen]
	}
	return childMatch
}
