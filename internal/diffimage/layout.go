package diffimage

import (
	"container/heap"
	"fmt"
	"image"
	"slices"
	"strings"
)

// layoutDiagram arranges the union graph once. Connected tables stay near each
// other; SQL names break every tie, independently of input array order or layout.
// Edges attach to column rows and route around the complete table rectangles,
// including change notes, so a relationship never disappears beneath a card.
func layoutDiagram(tables []*diagramTable, edges []diagramEdge) (int, int, error) {
	if len(tables) == 0 {
		return 0, 0, nil
	}
	if len(tables) > 192 || len(edges) > 1024 {
		return 0, 0, fmt.Errorf("ER diagram is too large (maximum 192 tables and 1024 relationships); use text output")
	}
	byName := make(map[string]*diagramTable, len(tables))
	neighbors := make(map[string][]string, len(tables))
	parents := make(map[string]int, len(tables))
	for _, table := range tables {
		byName[table.Name] = table
	}
	for _, edge := range edges {
		if byName[edge.From] == nil || byName[edge.To] == nil {
			return 0, 0, fmt.Errorf("relationship %q is missing a diagram endpoint", edge.Name)
		}
		if edge.From != edge.To {
			neighbors[edge.From] = append(neighbors[edge.From], edge.To)
			neighbors[edge.To] = append(neighbors[edge.To], edge.From)
			parents[edge.From]++
		}
	}
	for name, adjacent := range neighbors {
		slices.Sort(adjacent)
		neighbors[name] = slices.Compact(adjacent)
	}
	seeds := slices.Clone(tables)
	slices.SortFunc(seeds, func(a, b *diagramTable) int {
		if (parents[a.Name] == 0) != (parents[b.Name] == 0) {
			if parents[a.Name] == 0 {
				return -1
			}
			return 1
		}
		if difference := len(neighbors[b.Name]) - len(neighbors[a.Name]); difference != 0 {
			return difference
		}
		return strings.Compare(a.Name, b.Name)
	})
	ordered := make([]*diagramTable, 0, len(tables))
	visited := make(map[string]bool, len(tables))
	for _, seed := range seeds {
		if visited[seed.Name] {
			continue
		}
		queue := []string{seed.Name}
		visited[seed.Name] = true
		for len(queue) != 0 {
			name := queue[0]
			queue = queue[1:]
			ordered = append(ordered, byName[name])
			for _, adjacent := range neighbors[name] {
				if !visited[adjacent] {
					visited[adjacent] = true
					queue = append(queue, adjacent)
				}
			}
		}
	}

	const outer = 96
	gap := 144 + min(len(edges), 8)*6
	columns := min(3, len(tables))
	cellWidth := 0
	for _, table := range tables {
		cellWidth = max(cellWidth, table.Width)
	}
	width := outer*2 + columns*cellWidth + (columns-1)*gap
	y := outer
	for offset := 0; offset < len(ordered); offset += columns {
		rowHeight := 0
		for column, table := range ordered[offset:min(offset+columns, len(ordered))] {
			table.X, table.Y = outer+column*(cellWidth+gap), y
			rowHeight = max(rowHeight, table.Height)
		}
		y += rowHeight + gap
	}
	height := y - gap + outer
	if width > 8000 || height > 15000 || width*height > 32_000_000 {
		return 0, 0, fmt.Errorf("ER diagram exceeds the image size limit; use text output")
	}

	// Route in a stable order even if callers supply edges in a different order.
	indices := make([]int, len(edges))
	for i := range indices {
		indices[i] = i
	}
	slices.SortFunc(indices, func(a, b int) int {
		left, right := edges[a], edges[b]
		return strings.Compare(left.From+"\x00"+left.Name+"\x00"+left.Status+"\x00"+left.To+"\x00"+strings.Join(left.FromColumns, "\x00")+"\x00"+strings.Join(left.ToColumns, "\x00"), right.From+"\x00"+right.Name+"\x00"+right.Status+"\x00"+right.To+"\x00"+strings.Join(right.FromColumns, "\x00")+"\x00"+strings.Join(right.ToColumns, "\x00"))
	})
	ports := make(map[[2]image.Point]int)
	selfLanes := make(map[string]int)
	var routed [][]image.Point
	for lane, index := range indices {
		edge := &edges[index]
		edge.Points = nil
		source, target := byName[edge.From], byName[edge.To]
		if len(edge.FromColumns) == 0 || len(edge.ToColumns) == 0 {
			return 0, 0, fmt.Errorf("relationship %q is missing its column mapping", edge.Name)
		}
		sourceY, targetY := -1, -1
		for _, column := range source.Columns {
			if column.Name == edge.FromColumns[0] {
				sourceY = source.Y + column.Y + column.Height/2
			}
		}
		for _, column := range target.Columns {
			if column.Name == edge.ToColumns[0] {
				targetY = target.Y + column.Y + column.Height/2
			}
		}
		if sourceY < 0 || targetY < 0 {
			return 0, 0, fmt.Errorf("relationship %q refers to a missing diagram column", edge.Name)
		}
		clearance := 22 + lane%8*6
		start := image.Pt(source.X+source.Width, sourceY)
		end := image.Pt(target.X, targetY)
		startDirection, endDirection := 1, -1
		if source.X > target.X {
			start.X, end.X = source.X, target.X+target.Width
			startDirection, endDirection = -1, 1
		} else if source.X == target.X {
			end.X, endDirection = target.X+target.Width, 1
		}
		if source == target {
			// A loop remains visible even for a key referring to its own row.
			// Keep each loop distinct without exceeding the outer margin.
			if selfLanes[source.Name] >= 10 {
				return 0, 0, fmt.Errorf("too many self relationships on table %q; use text output", source.Name)
			}
			clearance = 22 + selfLanes[source.Name]*6
			selfLanes[source.Name]++
			edge.Points = []image.Point{start, image.Pt(start.X+clearance, start.Y), image.Pt(start.X+clearance, source.Y-clearance), image.Pt(start.X+clearance+16, source.Y-clearance), image.Pt(start.X+clearance+16, end.Y), end}
			routed = append(routed, edge.Points)
			continue
		}
		startStub := image.Pt(start.X+startDirection*clearance, start.Y)
		endStub := image.Pt(end.X+endDirection*clearance, end.Y)
		pair := [2]image.Point{start, end}
		if pair[1].X < pair[0].X || pair[1].X == pair[0].X && pair[1].Y < pair[0].Y {
			pair[0], pair[1] = pair[1], pair[0]
		}
		// Parallel and reciprocal relationships get separate lanes, rather than
		// overwriting one another with a different change colour.
		portOffset := ports[pair] * 9
		ports[pair]++
		if portOffset > 72 {
			return 0, 0, fmt.Errorf("too many relationships share the same column endpoints; use text output")
		}
		routeStart, routeEnd := startStub.Add(image.Pt(0, portOffset)), endStub.Add(image.Pt(0, portOffset))
		obstacles := make([]image.Rectangle, 0, len(tables))
		for _, table := range tables {
			obstacles = append(obstacles, image.Rect(table.X, table.Y, table.X+table.Width, table.Y+table.Height).Inset(-clearance))
		}
		middle, err := orthogonalRoute(routeStart, routeEnd, obstacles, routed, width, height)
		if err != nil {
			return 0, 0, fmt.Errorf("route relationship %q: %w", edge.Name, err)
		}
		points := append([]image.Point{start, startStub}, middle...)
		points = append(points, endStub, end)
		for _, point := range points {
			last := len(edge.Points) - 1
			if last >= 0 && edge.Points[last] == point {
				continue
			}
			if last > 0 && (edge.Points[last-1].X == point.X && edge.Points[last].X == point.X || edge.Points[last-1].Y == point.Y && edge.Points[last].Y == point.Y) {
				edge.Points[last] = point
			} else {
				edge.Points = append(edge.Points, point)
			}
		}
		routed = append(routed, edge.Points)
	}
	return width, height, nil
}

// orthogonalRoute uses the open corridors formed by the card boundaries. A*
// favours short routes with few bends; unlike a fixed elbow it handles an
// intervening card, a cycle, or tables on different rows without crossing them.
func orthogonalRoute(start, end image.Point, obstacles []image.Rectangle, routed [][]image.Point, width, height int) ([]image.Point, error) {
	xs, ys := []int{0, width, start.X, end.X}, []int{0, height, start.Y, end.Y}
	for _, obstacle := range obstacles {
		xs = append(xs, obstacle.Min.X, obstacle.Max.X)
		ys = append(ys, obstacle.Min.Y, obstacle.Max.Y)
	}
	slices.Sort(xs)
	xs = slices.Compact(xs)
	slices.Sort(ys)
	ys = slices.Compact(ys)
	sx, _ := slices.BinarySearch(xs, start.X)
	sy, _ := slices.BinarySearch(ys, start.Y)
	initial := (sy*len(xs) + sx) * 3
	distance := map[int]int{initial: 0}
	previous := map[int]int{initial: -1}
	pending := &routeQueue{{state: initial, score: abs(start.X-end.X) + abs(start.Y-end.Y)}}
	for pending.Len() > 0 {
		current := heap.Pop(pending).(routeStep)
		if distance[current.state] != current.cost {
			continue
		}
		cell, direction := current.state/3, current.state%3
		x, y := cell%len(xs), cell/len(xs)
		point := image.Pt(xs[x], ys[y])
		if point == end {
			var path []image.Point
			for state := current.state; state >= 0; state = previous[state] {
				cell := state / 3
				path = append(path, image.Pt(xs[cell%len(xs)], ys[cell/len(xs)]))
			}
			slices.Reverse(path)
			return path, nil
		}
		for _, step := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
			nx, ny := x+step[0], y+step[1]
			if nx < 0 || nx >= len(xs) || ny < 0 || ny >= len(ys) {
				continue
			}
			next := image.Pt(xs[nx], ys[ny])
			blocked := false
			for _, obstacle := range obstacles {
				if point.Y == next.Y {
					blocked = point.Y > obstacle.Min.Y && point.Y < obstacle.Max.Y && max(point.X, next.X) > obstacle.Min.X && min(point.X, next.X) < obstacle.Max.X
				} else {
					blocked = point.X > obstacle.Min.X && point.X < obstacle.Max.X && max(point.Y, next.Y) > obstacle.Min.Y && min(point.Y, next.Y) < obstacle.Max.Y
				}
				if blocked {
					break
				}
			}
			if blocked {
				continue
			}
			nextDirection := 1
			if nx == x {
				nextDirection = 2
			}
			cost := current.cost + abs(point.X-next.X) + abs(point.Y-next.Y)
			if direction != 0 && direction != nextDirection {
				cost += 24
			}
			// Prefer separate corridors so one change colour cannot cover another.
			// Crossings and short shared endpoint stubs remain possible.
			for _, route := range routed {
				for i := 1; i < len(route); i++ {
					a, b := route[i-1], route[i]
					if point.Y == next.Y && point.Y == a.Y && a.Y == b.Y {
						cost += 8 * max(0, min(max(point.X, next.X), max(a.X, b.X))-max(min(point.X, next.X), min(a.X, b.X)))
					} else if point.X == next.X && point.X == a.X && a.X == b.X {
						cost += 8 * max(0, min(max(point.Y, next.Y), max(a.Y, b.Y))-max(min(point.Y, next.Y), min(a.Y, b.Y)))
					}
				}
			}
			state := (ny*len(xs)+nx)*3 + nextDirection
			if oldCost, seen := distance[state]; seen && oldCost <= cost {
				continue
			}
			distance[state], previous[state] = cost, current.state
			heap.Push(pending, routeStep{state: state, cost: cost, score: cost + abs(next.X-end.X) + abs(next.Y-end.Y)})
		}
	}
	return nil, fmt.Errorf("no unobstructed route; use text output for this schema")
}

type routeStep struct{ state, cost, score int }
type routeQueue []routeStep

func (q routeQueue) Len() int { return len(q) }
func (q routeQueue) Less(i, j int) bool {
	if q[i].score != q[j].score {
		return q[i].score < q[j].score
	}
	return q[i].state < q[j].state
}
func (q routeQueue) Swap(i, j int)   { q[i], q[j] = q[j], q[i] }
func (q *routeQueue) Push(value any) { *q = append(*q, value.(routeStep)) }
func (q *routeQueue) Pop() any {
	last := (*q)[len(*q)-1]
	*q = (*q)[:len(*q)-1]
	return last
}
func abs(value int) int { return max(value, -value) }
