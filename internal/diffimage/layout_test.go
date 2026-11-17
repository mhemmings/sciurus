package diffimage

import (
	"fmt"
	"image"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDiagramRoutesColumnsWithoutCrossingTables(t *testing.T) {
	var tables []*diagramTable
	for _, name := range []string{"accounts", "organizations", "tasks", "comments", "members", "projects", "attachments"} {
		tables = append(tables, &diagramTable{Name: name, Width: 340, Height: 180, HeaderHeight: 48, Columns: []diagramColumn{{Name: "id", Y: 48, Height: 28}, {Name: "parent_id", Y: 76, Height: 28}}})
	}
	edges := []diagramEdge{
		{Name: "organization", From: "accounts", To: "organizations", FromColumns: []string{"parent_id"}, ToColumns: []string{"id"}},
		{Name: "account", From: "tasks", To: "accounts", FromColumns: []string{"parent_id"}, ToColumns: []string{"id"}},
		{Name: "task", From: "comments", To: "tasks", FromColumns: []string{"parent_id"}, ToColumns: []string{"id"}},
		{Name: "member", From: "comments", To: "members", FromColumns: []string{"id"}, ToColumns: []string{"id"}},
		{Name: "project", From: "members", To: "projects", FromColumns: []string{"parent_id"}, ToColumns: []string{"id"}},
		{Name: "cycle", From: "projects", To: "comments", FromColumns: []string{"parent_id"}, ToColumns: []string{"id"}},
		{Name: "attachment", From: "attachments", To: "accounts", FromColumns: []string{"parent_id"}, ToColumns: []string{"id"}},
		{Name: "self", From: "tasks", To: "tasks", FromColumns: []string{"parent_id"}, ToColumns: []string{"id"}},
	}
	width, height, err := layoutDiagram(tables, edges)
	require.NoError(t, err)
	byName := map[string]*diagramTable{}
	positions := map[string]image.Point{}
	for _, table := range tables {
		byName[table.Name] = table
		positions[table.Name] = image.Pt(table.X, table.Y)
		for _, other := range tables {
			if table != other {
				require.False(t, image.Rect(table.X, table.Y, table.X+table.Width, table.Y+table.Height).Overlaps(image.Rect(other.X, other.Y, other.X+other.Width, other.Y+other.Height)))
			}
		}
	}
	routes := map[string][]image.Point{}
	for _, edge := range edges {
		routes[edge.Name] = slices.Clone(edge.Points)
		require.GreaterOrEqual(t, len(edge.Points), 2)
		source, target := byName[edge.From], byName[edge.To]
		for endpoint, side := range []struct {
			table  *diagramTable
			column string
			point  image.Point
		}{{source, edge.FromColumns[0], edge.Points[0]}, {target, edge.ToColumns[0], edge.Points[len(edge.Points)-1]}} {
			for _, column := range side.table.Columns {
				if column.Name == side.column {
					require.Equal(t, side.table.Y+column.Y+column.Height/2, side.point.Y, "%s endpoint %d", edge.Name, endpoint)
				}
			}
			require.True(t, side.point.X == side.table.X || side.point.X == side.table.X+side.table.Width)
		}
		for i, point := range edge.Points {
			require.True(t, point.In(image.Rect(0, 0, width+1, height+1)), "%s: %v outside image", edge.Name, point)
			if i == 0 {
				continue
			}
			previous := edge.Points[i-1]
			require.True(t, previous.X == point.X || previous.Y == point.Y, "%s has diagonal segment", edge.Name)
			for _, table := range tables {
				interior := image.Rect(table.X+1, table.Y+1, table.X+table.Width, table.Y+table.Height)
				segment := image.Rect(min(point.X, previous.X), min(point.Y, previous.Y), max(point.X, previous.X)+1, max(point.Y, previous.Y)+1)
				require.False(t, interior.Overlaps(segment), "%s crosses %s", edge.Name, table.Name)
			}
		}
	}
	// Input storage order cannot rearrange the review or route its edges anew.
	slices.Reverse(tables)
	slices.Reverse(edges)
	_, _, err = layoutDiagram(tables, edges)
	require.NoError(t, err)
	for _, table := range tables {
		require.Equal(t, positions[table.Name], image.Pt(table.X, table.Y))
	}
	for _, edge := range edges {
		require.Equal(t, routes[edge.Name], edge.Points)
	}
}

func TestDiagramParallelAndSelfRelationshipsRemainVisible(t *testing.T) {
	tables := []*diagramTable{
		{Name: "a", Width: 300, Height: 110, Columns: []diagramColumn{{Name: "id", Y: 44, Height: 28}}},
		{Name: "b", Width: 300, Height: 110, Columns: []diagramColumn{{Name: "id", Y: 44, Height: 28}}},
	}
	edges := []diagramEdge{
		{Name: "first", From: "a", To: "b", FromColumns: []string{"id"}, ToColumns: []string{"id"}},
		{Name: "second", From: "a", To: "b", FromColumns: []string{"id"}, ToColumns: []string{"id"}},
		{Name: "reciprocal", From: "b", To: "a", FromColumns: []string{"id"}, ToColumns: []string{"id"}},
		{Name: "self", From: "a", To: "a", FromColumns: []string{"id"}, ToColumns: []string{"id"}},
	}
	_, _, err := layoutDiagram(tables, edges)
	require.NoError(t, err)
	require.NotEqual(t, edges[0].Points, edges[1].Points)
	reciprocal := slices.Clone(edges[2].Points)
	slices.Reverse(reciprocal)
	require.NotEqual(t, edges[0].Points, reciprocal)
	require.Equal(t, edges[3].Points[0], edges[3].Points[len(edges[3].Points)-1])
	require.Greater(t, len(slices.Compact(slices.Clone(edges[3].Points))), 4, "self reference needs a visible loop")
}

func TestDiagramDoesNotReuseSelfRelationshipLanes(t *testing.T) {
	tables := []*diagramTable{{Name: "tree", Width: 300, Height: 110, Columns: []diagramColumn{{Name: "id", Y: 44, Height: 28}}}}
	var edges []diagramEdge
	for i := 0; i < 10; i++ {
		edges = append(edges, diagramEdge{Name: fmt.Sprintf("self_%02d", i), From: "tree", To: "tree", FromColumns: []string{"id"}, ToColumns: []string{"id"}})
	}
	width, height, err := layoutDiagram(tables, edges)
	require.NoError(t, err)
	for i, edge := range edges {
		for _, other := range edges[:i] {
			require.NotEqual(t, other.Points, edge.Points, "self relationships must remain distinguishable")
		}
		for _, point := range edge.Points {
			require.True(t, point.In(image.Rect(0, 0, width, height)))
		}
	}
	edges = append(edges, diagramEdge{Name: "overflow", From: "tree", To: "tree", FromColumns: []string{"id"}, ToColumns: []string{"id"}})
	_, _, err = layoutDiagram(tables, edges)
	require.ErrorContains(t, err, "too many self relationships")
}

func TestDiagramKeepsDifferentRelationshipChangesVisible(t *testing.T) {
	var tables []*diagramTable
	for _, name := range []string{"organizations", "accounts", "legacy_tokens", "sessions"} {
		tables = append(tables, &diagramTable{Name: name, Width: 340, Height: 180, Columns: []diagramColumn{{Name: "id", Y: 48, Height: 28}, {Name: "parent_id", Y: 76, Height: 28}}})
	}
	edges := []diagramEdge{
		{Name: "organization", From: "accounts", To: "organizations", FromColumns: []string{"parent_id"}, ToColumns: []string{"id"}, Status: "modified"},
		{Name: "legacy_account", From: "legacy_tokens", To: "accounts", FromColumns: []string{"parent_id"}, ToColumns: []string{"id"}, Status: "removed"},
		{Name: "session_account", From: "sessions", To: "accounts", FromColumns: []string{"parent_id"}, ToColumns: []string{"id"}, Status: "added"},
	}
	_, _, err := layoutDiagram(tables, edges)
	require.NoError(t, err)
	// Parent columns in different cards share a Y coordinate. Their separate
	// relationships must not merge into one long, ambiguously coloured line.
	for i := 1; i < len(edges[0].Points); i++ {
		a, b := edges[0].Points[i-1], edges[0].Points[i]
		for j := 1; j < len(edges[2].Points); j++ {
			c, d := edges[2].Points[j-1], edges[2].Points[j]
			if a.Y == b.Y && a.Y == c.Y && c.Y == d.Y {
				overlap := min(max(a.X, b.X), max(c.X, d.X)) - max(min(a.X, b.X), min(c.X, d.X))
				require.LessOrEqual(t, overlap, 40, "only short endpoint stubs may overlap")
			}
		}
	}
}
