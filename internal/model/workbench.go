package model

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// workbenchNode retains the GRT properties and links shared by schema and
// diagram objects in document.mwb.xml.
type workbenchNode struct {
	ID         string          `xml:"id,attr"`
	Key        string          `xml:"key,attr"`
	StructName string          `xml:"struct-name,attr"`
	Text       string          `xml:",chardata"`
	Children   []workbenchNode `xml:",any"`
}

// field reads an immediate property, rather than a similarly named descendant.
func (node *workbenchNode) field(key string) string {
	property := node.property(key)
	if property != nil {
		return property.Text
	}
	return ""
}

func (node *workbenchNode) property(key string) *workbenchNode {
	for i := range node.Children {
		if node.Children[i].Key == key {
			return &node.Children[i]
		}
	}
	return nil
}

// number rejects malformed and non-finite coordinates before modifying a model.
func (node *workbenchNode) number(key string) (float64, error) {
	value, err := strconv.ParseFloat(node.field(key), 64)
	if err != nil || !finite(value) {
		return 0, fmt.Errorf("Workbench %s %q has invalid %s", node.StructName, node.ID, key)
	}
	return value, nil
}

// ApplyWorkbenchLayout copies the single diagram's layers and table positions
// from a MySQL Workbench ZIP archive. SQL remains the source of schema semantics:
// tables are matched by SQL name, and model-only Workbench foreign keys are never
// added. Workbench stores table coordinates relative to their enclosing layer;
// Sciurus stores absolute coordinates. The document changes only on success.
func ApplyWorkbenchLayout(doc *Document, data []byte) error {
	if doc == nil {
		return fmt.Errorf("cannot apply Workbench layout to a nil document")
	}
	archive, err := readWorkbench(data)
	if err != nil {
		return err
	}
	return archive.applyLayout(doc)
}

type workbenchDocument struct {
	objects map[string]*workbenchNode
	ordered []*workbenchNode
}

func readWorkbench(data []byte) (*workbenchDocument, error) {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("read Workbench archive: %w", err)
	}
	var document *zip.File
	for _, entry := range archive.File {
		if entry.Name == "document.mwb.xml" {
			if document != nil {
				return nil, fmt.Errorf("Workbench archive contains duplicate document.mwb.xml entries")
			}
			document = entry
		}
	}
	if document == nil {
		return nil, fmt.Errorf("Workbench archive is missing document.mwb.xml")
	}
	// No files are extracted. Bound decompression as well as the archive's stated
	// size so a compressed document cannot exhaust memory in the local web app.
	const maxXMLSize = 32 << 20
	if document.UncompressedSize64 > maxXMLSize {
		return nil, fmt.Errorf("Workbench XML exceeds the 32 MiB limit")
	}
	reader, err := document.Open()
	if err != nil {
		return nil, fmt.Errorf("open Workbench XML: %w", err)
	}
	contents, err := io.ReadAll(io.LimitReader(reader, maxXMLSize+1))
	closeErr := reader.Close()
	if err != nil {
		return nil, fmt.Errorf("read Workbench XML: %w", err)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close Workbench XML: %w", closeErr)
	}
	if len(contents) > maxXMLSize {
		return nil, fmt.Errorf("Workbench XML exceeds the 32 MiB limit")
	}
	var root workbenchNode
	err = xml.Unmarshal(contents, &root)
	if err != nil {
		return nil, fmt.Errorf("parse Workbench XML: %w", err)
	}
	// Preserve source order for deterministic group order, while indexing IDs for
	// the explicit table and layer links used throughout the GRT document.
	objects := make(map[string]*workbenchNode)
	var ordered []*workbenchNode
	stack := []*workbenchNode{&root}
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if node.ID != "" {
			if objects[node.ID] != nil {
				return nil, fmt.Errorf("Workbench XML has duplicate object ID %q", node.ID)
			}
			objects[node.ID] = node
			ordered = append(ordered, node)
		}
		for i := len(node.Children) - 1; i >= 0; i-- {
			stack = append(stack, &node.Children[i])
		}
	}
	return &workbenchDocument{objects: objects, ordered: ordered}, nil
}

func (archive *workbenchDocument) applyLayout(doc *Document) error {
	var diagram *workbenchNode
	for _, node := range archive.ordered {
		if node.StructName == "workbench.physical.Diagram" {
			if diagram != nil {
				return fmt.Errorf("Workbench layout import requires exactly one diagram")
			}
			diagram = node
		}
	}
	if diagram == nil {
		return fmt.Errorf("Workbench archive has no physical diagram")
	}
	updated := *doc
	updated.Tables = append([]Table(nil), doc.Tables...)
	updated.Groups = []Group{}
	tables := make(map[string]int, len(updated.Tables))
	for i := range updated.Tables {
		tables[updated.Tables[i].Name] = i
		updated.Tables[i].GroupID = ""
	}
	layerPositions := make(map[string]Point)
	layerGroups := make(map[string]string)
	for _, node := range archive.ordered {
		if node.StructName != "workbench.physical.Layer" || node.field("owner") != diagram.ID {
			continue
		}
		x, numberErr := node.number("left")
		if numberErr != nil {
			return numberErr
		}
		y, numberErr := node.number("top")
		if numberErr != nil {
			return numberErr
		}
		layerPositions[node.ID] = Point{X: x, Y: y}
		// The root layer is the canvas, not a user-visible group.
		if node.Key == "rootLayer" || node.field("name") == "" {
			continue
		}
		width, numberErr := node.number("width")
		if numberErr != nil {
			return numberErr
		}
		height, numberErr := node.number("height")
		if numberErr != nil {
			return numberErr
		}
		id := "group_" + strings.ToLower(strings.NewReplacer("{", "", "}", "", "-", "").Replace(node.ID))
		layerGroups[node.ID] = id
		color := node.field("color")
		if color == "" {
			color = "#94a3b8"
		}
		updated.Groups = append(updated.Groups, Group{ID: id, Name: node.field("name"), Color: color, Position: Point{X: x, Y: y}, Width: width, Height: height})
	}
	matched := make(map[string]bool)
	for _, node := range archive.ordered {
		if node.StructName != "workbench.physical.TableFigure" || node.field("owner") != diagram.ID {
			continue
		}
		table := archive.objects[node.field("table")]
		if table == nil {
			return fmt.Errorf("Workbench figure %q references a missing table", node.ID)
		}
		index, found := tables[table.field("name")]
		if !found {
			continue
		}
		if matched[table.field("name")] {
			return fmt.Errorf("Workbench diagram repeats table %q", table.field("name"))
		}
		matched[table.field("name")] = true
		origin, found := layerPositions[node.field("layer")]
		if !found {
			return fmt.Errorf("Workbench table %q references a missing layer", table.field("name"))
		}
		x, numberErr := node.number("left")
		if numberErr != nil {
			return numberErr
		}
		y, numberErr := node.number("top")
		if numberErr != nil {
			return numberErr
		}
		width, numberErr := node.number("width")
		if numberErr != nil {
			return numberErr
		}
		updated.Tables[index].Position = Point{X: origin.X + x, Y: origin.Y + y}
		updated.Tables[index].Width = width
		updated.Tables[index].GroupID = layerGroups[node.field("layer")]
	}
	if len(matched) == 0 {
		return fmt.Errorf("Workbench diagram has no table names matching the SQL model")
	}
	x, err := diagram.number("x")
	if err != nil {
		return err
	}
	y, err := diagram.number("y")
	if err != nil {
		return err
	}
	zoom, err := diagram.number("zoom")
	if err != nil {
		return err
	}
	updated.Viewport = Viewport{X: -x * zoom, Y: -y * zoom, Zoom: zoom}
	err = Validate(&updated)
	if err != nil {
		return fmt.Errorf("invalid imported Workbench layout: %w", err)
	}
	*doc = updated
	return nil
}
