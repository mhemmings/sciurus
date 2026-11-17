// Package diffimage renders database changes as a headless ER diagram. Fonts
// and layout are independent of the host and of either document's saved visuals.
package diffimage

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"slices"
	"strings"
	"unicode"

	"github.com/mhemmings/sciurus/internal/model"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

const (
	imageWidth     = 1440
	maxImageHeight = 16000
	margin         = 56
	cardWidth      = 480
	lineHeight     = 24
)

var (
	background = color.RGBA{245, 247, 251, 255}
	surface    = color.RGBA{255, 255, 255, 255}
	ink        = color.RGBA{27, 38, 57, 255}
	muted      = color.RGBA{94, 109, 131, 255}
	border     = color.RGBA{212, 221, 234, 255}
	blue       = color.RGBA{49, 88, 163, 255}
	green      = color.RGBA{21, 116, 72, 255}
	greenWash  = color.RGBA{232, 247, 239, 255}
	red        = color.RGBA{175, 48, 61, 255}
	redWash    = color.RGBA{255, 237, 239, 255}
	amber      = color.RGBA{145, 91, 17, 255}
	amberWash  = color.RGBA{255, 247, 226, 255}
)

type reportFonts struct {
	title, table, body, bold, small, mono font.Face
}

// Columns are anchored using their main row, excluding wrapped annotations.
// X/Y and edge Points are graph-local; the renderer adds the page header.
type diagramTable struct {
	Name                              string
	X, Y, Width, Height, HeaderHeight int
	Columns                           []diagramColumn
	Status                            string
	Title                             []string
	Notes                             []diagramNote
}

type diagramColumn struct {
	Name                        string
	Y, Height, FullHeight       int
	Status, Markers             string
	NameLines, TypeLines, Notes []string
	Nullable                    bool
}

type diagramNote struct {
	Status string
	Lines  []string
}

type diagramEdge struct {
	Name, From, To         string
	FromColumns, ToColumns []string
	Status                 string
	Points                 []image.Point
	UniqueSource           bool
	OptionalReference      bool
}

// Render draws changed tables and their immediate relationship neighbours in
// one union diagram. Additions, removals and modifications have explicit marks;
// hidden database properties are annotated beside their affected object.
func Render(diff *model.Diff, beforeLabel, afterLabel string) ([]byte, error) {
	if diff == nil {
		return nil, errors.New("cannot render a nil schema diff")
	}
	f, err := loadFonts()
	if err != nil {
		return nil, err
	}
	defer func() {
		for _, face := range []font.Face{f.title, f.table, f.body, f.bold, f.small, f.mono} {
			_ = face.Close()
		}
	}()
	tables, edges, err := buildDiagram(diff, f)
	if err != nil {
		return nil, err
	}
	graphWidth, graphHeight := 0, 200
	if len(tables) > 0 {
		graphWidth, graphHeight, err = layoutDiagram(tables, edges)
		if err != nil {
			return nil, err
		}
	}
	width := max(imageWidth, graphWidth+2*margin)
	before, err := wrapText(beforeLabel, f.body, (width-2*margin)/2-36)
	if err != nil {
		return nil, err
	}
	after, err := wrapText(afterLabel, f.body, (width-2*margin)/2-36)
	if err != nil {
		return nil, err
	}
	headerHeight := 244 + max(1, len(before), len(after))*lineHeight
	height := headerHeight + graphHeight + 96
	if width > 8000 || height > maxImageHeight || int64(width)*int64(height) > 32_000_000 {
		return nil, reportTooLarge()
	}
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	p := painter{image: canvas, fonts: f}
	p.fill(canvas.Bounds(), background)
	for y := headerHeight; y < height-44; y += 20 {
		for x := 16; x < width; x += 20 {
			p.fill(image.Rect(x, y, x+1, y+1), border)
		}
	}
	p.text(margin, 46, "SCIURUS  /  DATABASE CHANGES", f.small, blue)
	p.text(margin, 92, "Schema diff", f.title, ink)
	summary := "Schemas match — no database changes"
	if len(diff.Tables) > 0 {
		summary = fmt.Sprintf("%d changed %s", len(diff.Tables), plural(len(diff.Tables), "table", "tables"))
		if neighbours := len(tables) - len(diff.Tables); neighbours > 0 {
			summary += fmt.Sprintf("  /  %d related %s for context", neighbours, plural(neighbours, "table", "tables"))
		}
	}
	p.text(margin, 125, summary, f.body, muted)
	x := margin
	for _, legend := range []struct{ label, status string }{{"+  Added", "added"}, {"−  Removed", "removed"}, {"~  Modified", "modified"}, {"Unchanged context", ""}} {
		fg, bg, _ := statusStyle(legend.status)
		w := font.MeasureString(f.body, legend.label).Ceil() + 26
		p.rounded(image.Rect(x, 145, x+w, 179), 5, bg)
		p.text(x+13, 168, legend.label, f.body, fg)
		x += w + 14
	}
	secondX := width / 2
	p.text(margin, 214, "BEFORE", f.small, muted)
	p.text(secondX, 214, "AFTER", f.small, muted)
	p.lines(margin, 240, before, f.body, ink)
	p.lines(secondX, 240, after, f.body, ink)
	offset := image.Pt((width-graphWidth)/2, headerHeight+20)
	if len(tables) == 0 {
		p.rounded(image.Rect(margin, headerHeight+24, width-margin, headerHeight+164), 8, surface)
		p.text(margin+28, headerHeight+78, "Schemas match", f.table, green)
		p.text(margin+28, headerHeight+114, "Both inputs describe the same database schema.", f.body, ink)
	}
	for _, highlighted := range []bool{false, true} {
		for _, edge := range edges {
			if (edge.Status != "") == highlighted {
				p.drawEdge(edge, offset)
			}
		}
	}
	for _, table := range tables {
		p.drawTable(table, offset)
	}
	p.text(margin, height-27, "PK  Primary key     FK  Foreign key     *  NOT NULL     /     Backslashes and unsupported font characters are escaped", f.small, muted)
	var output bytes.Buffer
	if err := png.Encode(&output, canvas); err != nil {
		return nil, fmt.Errorf("encode schema diff PNG: %w", err)
	}
	return output.Bytes(), nil
}

func buildDiagram(diff *model.Diff, f reportFonts) ([]*diagramTable, []diagramEdge, error) {
	before, after := map[string]model.SchemaTable{}, map[string]model.SchemaTable{}
	for _, table := range diff.BeforeSchema.Tables {
		before[table.Name] = table
	}
	for _, table := range diff.AfterSchema.Tables {
		after[table.Name] = table
	}
	changes := map[string]model.TableDiff{}
	selected := map[string]bool{}
	for _, change := range diff.Tables {
		changes[change.Name] = change
		selected[change.Name] = true
	}
	for _, graph := range []model.SchemaGraph{diff.BeforeSchema, diff.AfterSchema} {
		for _, table := range graph.Tables {
			for _, key := range table.ForeignKeys {
				_, sourceChanged := changes[table.Name]
				_, targetChanged := changes[key.ReferencedTable]
				if sourceChanged || targetChanged {
					selected[table.Name], selected[key.ReferencedTable] = true, true
				}
			}
		}
	}
	names := make([]string, 0, len(selected))
	for name := range selected {
		names = append(names, name)
	}
	slices.Sort(names)
	var tables []*diagramTable
	var edges []diagramEdge
	for _, name := range names {
		old, oldExists := before[name]
		next, nextExists := after[name]
		if !oldExists && !nextExists {
			return nil, nil, fmt.Errorf("schema diff is missing diagram data for table %q", name)
		}
		table := &diagramTable{Name: name, Width: cardWidth, Status: changes[name].Kind}
		var err error
		table.Title, err = wrapText(name, f.table, cardWidth-74)
		if err != nil {
			return nil, nil, err
		}
		table.HeaderHeight = 28 + len(table.Title)*28
		table.Height = table.HeaderHeight
		oldColumns, nextColumns := map[string]model.SchemaColumn{}, map[string]model.SchemaColumn{}
		columnNames := []string{}
		for _, column := range next.Columns {
			nextColumns[column.Name] = column
			columnNames = append(columnNames, column.Name)
		}
		for _, column := range old.Columns {
			oldColumns[column.Name] = column
			if _, exists := nextColumns[column.Name]; !exists {
				columnNames = append(columnNames, column.Name)
			}
		}
		for _, columnName := range columnNames {
			previous, had := oldColumns[columnName]
			current, has := nextColumns[columnName]
			column := diagramColumn{Name: columnName, Y: table.Height}
			display := current
			switch {
			case !had:
				column.Status = "added"
			case !has:
				column.Status, display = "removed", previous
			case previous.Definition != current.Definition || previous.Primary != current.Primary || previous.Foreign != current.Foreign:
				column.Status = "modified"
			}
			column.Nullable = display.Nullable
			if display.Primary {
				column.Markers = "PK"
			}
			if display.Foreign {
				column.Markers += " FK"
			}
			column.NameLines, err = wrapText(columnName, f.bold, 194)
			if err != nil {
				return nil, nil, err
			}
			typeLabel := display.Type
			if had && has && previous.Type != current.Type {
				typeLabel = previous.Type + " → " + current.Type
			}
			column.TypeLines, err = wrapText(typeLabel, f.mono, 163)
			if err != nil {
				return nil, nil, err
			}
			column.Height = max(1, len(column.NameLines), len(column.TypeLines))*lineHeight + 16
			annotations := []string{}
			if had && has && previous.Nullable != current.Nullable {
				from, to := "NULL", "NOT NULL"
				if current.Nullable {
					from, to = to, from
				}
				annotations = append(annotations, from+" → "+to)
			}
			if had && has && (previous.Primary != current.Primary || previous.Foreign != current.Foreign) {
				roles := make([]string, 0, 2)
				for _, value := range []model.SchemaColumn{previous, current} {
					role := []string{}
					if value.Primary {
						role = append(role, "PK")
					}
					if value.Foreign {
						role = append(role, "FK")
					}
					label := strings.Join(role, "/")
					if label == "" {
						label = "none"
					}
					roles = append(roles, label)
				}
				annotations = append(annotations, "key role: "+strings.Join(roles, " → "))
			}
			attributeNames := []string{}
			for attr := range previous.Attributes {
				attributeNames = append(attributeNames, attr)
			}
			for attr := range current.Attributes {
				if _, exists := previous.Attributes[attr]; !exists {
					attributeNames = append(attributeNames, attr)
				}
			}
			slices.Sort(attributeNames)
			for _, attr := range attributeNames {
				from, to := previous.Attributes[attr], current.Attributes[attr]
				if had && has && from == to {
					continue
				}
				value := to
				if !has {
					value = from
				} else if had {
					if from == "" {
						from = "none"
					}
					if to == "" {
						to = "none"
					}
					value = from + " → " + to
				}
				annotations = append(annotations, attr+": "+value)
			}
			for _, annotation := range annotations {
				lines, err := wrapText(annotation, f.body, cardWidth-90)
				if err != nil {
					return nil, nil, err
				}
				column.Notes = append(column.Notes, lines...)
			}
			column.FullHeight = column.Height
			if len(column.Notes) > 0 {
				column.FullHeight += len(column.Notes)*lineHeight + 8
			}
			table.Columns = append(table.Columns, column)
			table.Height += column.FullHeight
		}

		oldKeys, nextKeys := map[string]model.SchemaForeignKey{}, map[string]model.SchemaForeignKey{}
		keyNames := []string{}
		for _, key := range old.ForeignKeys {
			oldKeys[key.Name] = key
			keyNames = append(keyNames, key.Name)
		}
		for _, key := range next.ForeignKeys {
			nextKeys[key.Name] = key
			if _, exists := oldKeys[key.Name]; !exists {
				keyNames = append(keyNames, key.Name)
			}
		}
		slices.Sort(keyNames)
		for _, keyName := range keyNames {
			previous, had := oldKeys[keyName]
			current, has := nextKeys[keyName]
			previousOptional, currentOptional := false, false
			for _, column := range previous.Columns {
				previousOptional = previousOptional || oldColumns[column].Nullable
			}
			for _, column := range current.Columns {
				currentOptional = currentOptional || nextColumns[column].Nullable
			}
			status := ""
			display := current
			optional := currentOptional
			switch {
			case !had:
				status = "added"
			case !has:
				status, display = "removed", previous
				optional = previousOptional
			case previous.Definition != current.Definition || previous.UniqueSource != current.UniqueSource || previousOptional != currentOptional:
				status = "modified"
			}
			moved := had && has && (previous.ReferencedTable != current.ReferencedTable || !slices.Equal(previous.Columns, current.Columns) || !slices.Equal(previous.ReferencedColumns, current.ReferencedColumns))
			variants := []struct {
				key      model.SchemaForeignKey
				status   string
				optional bool
			}{{display, status, optional}}
			if moved {
				variants = []struct {
					key      model.SchemaForeignKey
					status   string
					optional bool
				}{{previous, "removed", previousOptional}, {current, "added", currentOptional}}
			}
			for _, variant := range variants {
				key := variant.key
				if selected[key.ReferencedTable] {
					edges = append(edges, diagramEdge{Name: key.Name, From: name, To: key.ReferencedTable, FromColumns: key.Columns, ToColumns: key.ReferencedColumns, Status: variant.status, UniqueSource: key.UniqueSource, OptionalReference: variant.optional})
				}
				note := key.Name + ": (" + strings.Join(key.Columns, ", ") + ") → " + key.ReferencedTable + " (" + strings.Join(key.ReferencedColumns, ", ") + ")"
				if had && has && !moved && previous.UniqueSource != current.UniqueSource {
					from, to := "1:N", "1:1"
					if !current.UniqueSource {
						from, to = to, from
					}
					note += "; " + from + " → " + to
				}
				if had && has && !moved && previousOptional != currentOptional {
					from, to := "required", "optional"
					if !currentOptional {
						from, to = to, from
					}
					note += "; reference " + from + " → " + to
				}
				for _, rule := range []struct{ name, old, next string }{{"delete", previous.OnDelete, key.OnDelete}, {"update", previous.OnUpdate, key.OnUpdate}} {
					if had && has && !moved && rule.old != rule.next {
						note += "; " + rule.name + " " + actionName(rule.old) + " → " + actionName(rule.next)
					} else if rule.next != "" && strings.ToUpper(rule.next) != "RESTRICT" && strings.ToUpper(rule.next) != "NO ACTION" {
						note += "; " + rule.name + " " + strings.ToUpper(rule.next)
					}
				}
				lines, err := wrapText(note, f.body, cardWidth-48)
				if err != nil {
					return nil, nil, err
				}
				table.Notes = append(table.Notes, diagramNote{Status: variant.status, Lines: lines})
			}
		}
		for _, change := range changes[name].Changes {
			if change.Kind == "column" || change.Kind == "foreign key" {
				continue
			}
			status, note := "modified", change.Before+" → "+change.After
			if change.Before == "" {
				status, note = "added", change.After
			} else if change.After == "" {
				status, note = "removed", change.Before
			}
			lines, err := wrapText(change.Kind+": "+note, f.body, cardWidth-48)
			if err != nil {
				return nil, nil, err
			}
			table.Notes = append(table.Notes, diagramNote{Status: status, Lines: lines})
		}
		for _, note := range table.Notes {
			table.Height += len(note.Lines)*lineHeight + 20
		}
		table.Height += 8
		if table.Height > maxImageHeight {
			return nil, nil, reportTooLarge()
		}
		tables = append(tables, table)
	}
	return tables, edges, nil
}

func actionName(value string) string {
	if value == "" {
		return "RESTRICT"
	}
	return strings.ToUpper(value)
}

func statusStyle(status string) (color.RGBA, color.RGBA, string) {
	switch status {
	case "added":
		return green, greenWash, "+"
	case "removed":
		return red, redWash, "−"
	case "modified":
		return amber, amberWash, "~"
	default:
		return muted, color.RGBA{235, 240, 248, 255}, ""
	}
}

func loadFonts() (reportFonts, error) {
	regular, err := opentype.Parse(goregular.TTF)
	if err != nil {
		return reportFonts{}, err
	}
	bold, err := opentype.Parse(gobold.TTF)
	if err != nil {
		return reportFonts{}, err
	}
	mono, err := opentype.Parse(gomono.TTF)
	if err != nil {
		return reportFonts{}, err
	}
	var fonts reportFonts
	for _, item := range []struct {
		target *font.Face
		font   *opentype.Font
		size   float64
	}{{&fonts.title, bold, 38}, {&fonts.table, bold, 22}, {&fonts.body, regular, 16}, {&fonts.bold, bold, 17}, {&fonts.small, bold, 13}, {&fonts.mono, mono, 16}} {
		*item.target, err = opentype.NewFace(item.font, &opentype.FaceOptions{Size: item.size, DPI: 72, Hinting: font.HintingFull})
		if err != nil {
			return reportFonts{}, err
		}
	}
	return fonts, nil
}

// wrapText retains content while making invisible or unsupported characters
// explicit. Literal backslashes are escaped to distinguish them from fallbacks.
func wrapText(text string, face font.Face, width int) ([]string, error) {
	if text == "" {
		return nil, nil
	}
	var visible strings.Builder
	for _, r := range text {
		_, supported := face.GlyphAdvance(r)
		switch {
		case r == '\n':
			visible.WriteRune(r)
		case r == '\\':
			visible.WriteString(`\\`)
		case !supported || unicode.IsControl(r) || unicode.Is(unicode.Cf, r):
			if r <= 0xffff {
				fmt.Fprintf(&visible, "\\u%04X", r)
			} else {
				fmt.Fprintf(&visible, "\\U%08X", r)
			}
		default:
			visible.WriteRune(r)
		}
	}
	var lines []string
	for _, paragraph := range strings.Split(visible.String(), "\n") {
		runes := []rune(paragraph)
		if len(runes) == 0 {
			lines = append(lines, "")
			if len(lines) > maxImageHeight/lineHeight {
				return nil, reportTooLarge()
			}
		}
		for len(runes) > 0 {
			end, wordBreak := 0, 0
			var measured fixed.Int26_6
			for end < len(runes) {
				advance, _ := face.GlyphAdvance(runes[end])
				if end > 0 {
					advance += face.Kern(runes[end-1], runes[end])
				}
				if measured+advance > fixed.I(width) && end > 0 {
					break
				}
				measured += advance
				end++
				if unicode.IsSpace(runes[end-1]) {
					wordBreak = end
				}
			}
			if end < len(runes) && wordBreak > 0 {
				end = wordBreak
			}
			lines = append(lines, string(runes[:end]))
			runes = runes[end:]
			if len(lines) > maxImageHeight/lineHeight {
				return nil, reportTooLarge()
			}
		}
	}
	return lines, nil
}

func reportTooLarge() error {
	return errors.New("schema diff diagram exceeds the image size limit; use text output for this diff")
}

func plural(count int, singular, plural string) string {
	if count == 1 {
		return singular
	}
	return plural
}

type painter struct {
	image *image.RGBA
	fonts reportFonts
}

func (p painter) fill(rect image.Rectangle, c color.RGBA) {
	draw.Draw(p.image, rect, image.NewUniform(c), image.Point{}, draw.Src)
}

func (p painter) rounded(rect image.Rectangle, radius float32, c color.RGBA) {
	w, h := float32(rect.Dx()), float32(rect.Dy())
	r := vector.NewRasterizer(rect.Dx(), rect.Dy())
	r.MoveTo(radius, 0)
	r.LineTo(w-radius, 0)
	r.QuadTo(w, 0, w, radius)
	r.LineTo(w, h-radius)
	r.QuadTo(w, h, w-radius, h)
	r.LineTo(radius, h)
	r.QuadTo(0, h, 0, h-radius)
	r.LineTo(0, radius)
	r.QuadTo(0, 0, radius, 0)
	r.ClosePath()
	r.Draw(p.image, rect, image.NewUniform(c), image.Point{})
}

func (p painter) text(x, baseline int, value string, face font.Face, c color.RGBA) {
	d := font.Drawer{Dst: p.image, Src: image.NewUniform(c), Face: face, Dot: fixed.P(x, baseline)}
	d.DrawString(value)
}

func (p painter) lines(x, baseline int, lines []string, face font.Face, c color.RGBA) {
	for _, line := range lines {
		p.text(x, baseline, line, face, c)
		baseline += lineHeight
	}
}

func (p painter) drawTable(table *diagramTable, offset image.Point) {
	x, y := table.X+offset.X, table.Y+offset.Y
	fg, bg, mark := statusStyle(table.Status)
	p.rounded(image.Rect(x+2, y+3, x+table.Width+2, y+table.Height+3), 8, color.RGBA{222, 228, 238, 255})
	p.rounded(image.Rect(x, y, x+table.Width, y+table.Height), 8, border)
	p.rounded(image.Rect(x+1, y+1, x+table.Width-1, y+table.Height-1), 7, surface)
	p.rounded(image.Rect(x+1, y+1, x+table.Width-1, y+table.HeaderHeight+6), 7, bg)
	p.fill(image.Rect(x+1, y+table.HeaderHeight-1, x+table.Width-1, y+table.HeaderHeight+7), surface)
	for i, title := range table.Title {
		p.text(x+18, y+35+i*28, title, p.fonts.table, ink)
	}
	p.text(x+table.Width-38, y+35, mark, p.fonts.table, fg)
	p.fill(image.Rect(x+1, y+table.HeaderHeight-1, x+table.Width-1, y+table.HeaderHeight), border)
	for _, column := range table.Columns {
		rowY := y + column.Y
		colFG, colBG, colMark := statusStyle(column.Status)
		if column.Status != "" {
			p.fill(image.Rect(x+1, rowY, x+table.Width-1, rowY+column.FullHeight), colBG)
		}
		p.text(x+10, rowY+27, colMark, p.fonts.bold, colFG)
		for i, marker := range strings.Fields(column.Markers) {
			p.text(x+34, rowY+25+i*15, marker, p.fonts.small, blue)
		}
		p.lines(x+75, rowY+27, column.NameLines, p.fonts.bold, ink)
		if !column.Nullable {
			p.text(x+284, rowY+27, "*", p.fonts.bold, muted)
		}
		p.lines(x+300, rowY+27, column.TypeLines, p.fonts.mono, colFG)
		p.lines(x+75, rowY+column.Height+16, column.Notes, p.fonts.body, colFG)
		p.fill(image.Rect(x+15, rowY+column.FullHeight-1, x+table.Width-15, rowY+column.FullHeight), border)
	}
	noteY := y + table.HeaderHeight
	for _, column := range table.Columns {
		noteY += column.FullHeight
	}
	for _, note := range table.Notes {
		fg, _, mark := statusStyle(note.Status)
		p.text(x+10, noteY+26, mark, p.fonts.bold, fg)
		p.lines(x+26, noteY+26, note.Lines, p.fonts.body, fg)
		noteY += len(note.Lines)*lineHeight + 20
	}
}

func (p painter) drawEdge(edge diagramEdge, offset image.Point) {
	if len(edge.Points) < 2 {
		return
	}
	fg, bg, mark := statusStyle(edge.Status)
	if edge.Status == "" {
		fg = color.RGBA{130, 149, 178, 255}
	}
	points := make([]image.Point, len(edge.Points))
	for i, point := range edge.Points {
		points[i] = point.Add(offset)
	}
	longest, midpoint := 0, image.Point{}
	for i := 1; i < len(points); i++ {
		a, b := points[i-1], points[i]
		p.stroke(a, b, 5, background, false)
		p.stroke(a, b, 2, fg, edge.Status == "removed")
		length := abs(a.X-b.X) + abs(a.Y-b.Y)
		if length > longest {
			longest, midpoint = length, image.Pt((a.X+b.X)/2, (a.Y+b.Y)/2)
		}
	}
	for _, end := range []struct {
		point, next    image.Point
		many, optional bool
	}{{points[0], points[1], !edge.UniqueSource, true}, {points[len(points)-1], points[len(points)-2], false, edge.OptionalReference}} {
		dx, dy := sign(end.next.X-end.point.X), sign(end.next.Y-end.point.Y)
		center := end.point.Add(image.Pt(dx*13, dy*13))
		if end.many {
			for _, spread := range []int{-7, 0, 7} {
				p.stroke(end.point.Add(image.Pt(-dy*spread, dx*spread)), center, 2, fg, false)
			}
		} else {
			distances := []int{7, 12}
			if end.optional {
				distances = distances[:1]
			}
			for _, distance := range distances {
				center := end.point.Add(image.Pt(dx*distance, dy*distance))
				p.stroke(center.Add(image.Pt(-dy*7, dx*7)), center.Add(image.Pt(dy*7, -dx*7)), 2, fg, false)
			}
		}
		if end.optional {
			circle := end.point.Add(image.Pt(dx*22, dy*22))
			p.rounded(image.Rect(circle.X-5, circle.Y-5, circle.X+6, circle.Y+6), 5.5, fg)
			p.rounded(image.Rect(circle.X-3, circle.Y-3, circle.X+4, circle.Y+4), 3.5, background)
		}
	}
	if mark != "" {
		p.rounded(image.Rect(midpoint.X-11, midpoint.Y-13, midpoint.X+12, midpoint.Y+13), 5, bg)
		p.text(midpoint.X-5, midpoint.Y+6, mark, p.fonts.bold, fg)
	}
}

func (p painter) stroke(a, b image.Point, thickness int, c color.RGBA, dashed bool) {
	dx, dy := abs(b.X-a.X), -abs(b.Y-a.Y)
	sx, sy := sign(b.X-a.X), sign(b.Y-a.Y)
	err, step := dx+dy, 0
	for {
		if !dashed || step%14 < 9 {
			p.fill(image.Rect(a.X-thickness/2, a.Y-thickness/2, a.X+(thickness+1)/2, a.Y+(thickness+1)/2), c)
		}
		if a == b {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			a.X += sx
		}
		if e2 <= dx {
			err += dx
			a.Y += sy
		}
		step++
	}
}

func sign(value int) int {
	if value < 0 {
		return -1
	}
	if value > 0 {
		return 1
	}
	return 0
}
