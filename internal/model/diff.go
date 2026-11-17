package model

import (
	"fmt"
	"slices"
	"strings"

	"vitess.io/vitess/go/vt/sqlparser"
)

// Diff contains database changes only, sorted by SQL table name. Object IDs,
// document titles and diagram metadata never participate in the comparison.
type Diff struct {
	Tables       []TableDiff
	BeforeSchema SchemaGraph
	AfterSchema  SchemaGraph
}

// SchemaGraph is the complete canonical database graph for diagram rendering,
// including unchanged tables. It contains SQL names, never saved IDs or layout.
type SchemaGraph struct {
	Tables []SchemaTable
}

// SchemaTable supplies a diagram's ordered column rows and named relationships.
type SchemaTable struct {
	Name        string
	Columns     []SchemaColumn
	ForeignKeys []SchemaForeignKey
}

// SchemaColumn separates compact diagram labels from the full SQL definition.
// Attributes contains non-default SQL attributes, keyed by their display label.
type SchemaColumn struct {
	Name       string
	Type       string
	Nullable   bool
	Primary    bool
	Foreign    bool
	Definition string
	Attributes map[string]string
}

// SchemaForeignKey connects SQL column names independently of diagram routing.
// UniqueSource distinguishes a one-to-one relationship from a one-to-many one.
type SchemaForeignKey struct {
	Name              string
	Columns           []string
	ReferencedTable   string
	ReferencedColumns []string
	OnDelete          string
	OnUpdate          string
	UniqueSource      bool
	Definition        string
}

// TableDiff describes an added, removed or modified table. Before and After
// contain canonical CREATE TABLE statements; an absent table has an empty SQL
// string. Names identify database objects, so renames are removal/addition pairs.
type TableDiff struct {
	Name    string
	Kind    string
	Before  string
	After   string
	Changes []DefinitionDiff
}

// DefinitionDiff describes a column, index, foreign key, check, table options,
// column order or clustered key change. Before and After are full definitions.
type DefinitionDiff struct {
	Kind   string
	Name   string
	Before string
	After  string
}

// HasChanges reports whether either schema has a different database definition.
func (diff *Diff) HasChanges() bool { return len(diff.Tables) != 0 }

type schemaDefinition struct {
	kind string
	name string
	sql  string
}

type schemaTable struct {
	name         string
	sql          string
	definitions  []schemaDefinition
	columnOrder  string
	clusteredKey string
	graph        SchemaTable
}

// Compare compares valid documents through their generated database schema.
// SQL parsing normalizes formatting and supported syntax aliases without a
// database connection. Unknown server defaults and equivalent-but-differently
// written expressions remain distinct; this is not a SQL equivalence prover.
func Compare(before, after *Document) (*Diff, error) {
	previous, err := canonicalSchema(before)
	if err != nil {
		return nil, fmt.Errorf("before schema: %w", err)
	}
	next, err := canonicalSchema(after)
	if err != nil {
		return nil, fmt.Errorf("after schema: %w", err)
	}
	oldTables, newTables := map[string]schemaTable{}, map[string]schemaTable{}
	names := []string{}
	diff := &Diff{Tables: []TableDiff{}, BeforeSchema: SchemaGraph{Tables: []SchemaTable{}}, AfterSchema: SchemaGraph{Tables: []SchemaTable{}}}
	for _, table := range previous {
		oldTables[table.name] = table
		names = append(names, table.name)
		diff.BeforeSchema.Tables = append(diff.BeforeSchema.Tables, table.graph)
	}
	for _, table := range next {
		newTables[table.name] = table
		diff.AfterSchema.Tables = append(diff.AfterSchema.Tables, table.graph)
		if _, exists := oldTables[table.name]; !exists {
			names = append(names, table.name)
		}
	}
	slices.Sort(names)
	for _, name := range names {
		oldTable, oldExists := oldTables[name]
		newTable, newExists := newTables[name]
		if oldTable.sql == newTable.sql {
			continue
		}
		change := TableDiff{Name: name, Kind: "modified", Before: oldTable.sql, After: newTable.sql, Changes: []DefinitionDiff{}}
		switch {
		case !oldExists:
			change.Kind = "added"
		case !newExists:
			change.Kind = "removed"
		}
		remaining := map[string]schemaDefinition{}
		for _, definition := range newTable.definitions {
			remaining[definition.kind+"\x00"+definition.name] = definition
		}
		for _, definition := range oldTable.definitions {
			key := definition.kind + "\x00" + definition.name
			replacement := remaining[key]
			if definition.sql != replacement.sql {
				change.Changes = append(change.Changes, DefinitionDiff{Kind: definition.kind, Name: definition.name, Before: definition.sql, After: replacement.sql})
			}
			delete(remaining, key)
		}
		for _, definition := range newTable.definitions {
			if _, exists := remaining[definition.kind+"\x00"+definition.name]; exists {
				change.Changes = append(change.Changes, DefinitionDiff{Kind: definition.kind, Name: definition.name, After: definition.sql})
			}
		}
		if oldExists && newExists && oldTable.columnOrder != newTable.columnOrder {
			change.Changes = append(change.Changes, DefinitionDiff{Kind: "column order", Name: "columns", Before: oldTable.columnOrder, After: newTable.columnOrder})
		}
		if oldExists && newExists && oldTable.clusteredKey != newTable.clusteredKey {
			change.Changes = append(change.Changes, DefinitionDiff{Kind: "clustered key", Name: "InnoDB clustered key", Before: oldTable.clusteredKey, After: newTable.clusteredKey})
		}
		diff.Tables = append(diff.Tables, change)
	}
	return diff, nil
}

// canonicalSchema uses the existing SQL generator to resolve IDs and inherited
// document options, then the parser to canonicalize complete definitions. It
// never changes the input document. Only columns and key parts retain order.
func canonicalSchema(document *Document) ([]schemaTable, error) {
	if err := Validate(document); err != nil {
		return nil, err
	}
	parser := sqlparser.NewTestParser()
	tables := make([]schemaTable, 0, len(document.Tables))
	for _, table := range document.Tables {
		statement, err := parser.ParseStrictDDL(tableSQL(document, &table))
		if err != nil {
			return nil, fmt.Errorf("table %q: %w", table.Name, err)
		}
		create := statement.(*sqlparser.CreateTable)
		spec := create.TableSpec
		charset, collation := "", ""
		for _, option := range spec.Options {
			option.Name = strings.ToLower(option.Name)
			switch option.Name {
			case "engine", "charset", "collate":
				option.String = strings.ToLower(unquoteOption(option.String))
				if option.Name == "charset" {
					charset = option.String
				} else if option.Name == "collate" {
					collation = option.String
				}
			}
		}
		if charset == "" && collation != "" {
			charset = strings.SplitN(collation, "_", 2)[0]
			spec.Options = append(spec.Options, &sqlparser.TableOption{Name: "charset", String: charset})
		}
		columnNames := map[string]string{}
		columns := map[string]*sqlparser.ColumnDefinition{}
		for _, column := range spec.Columns {
			columnNames[strings.ToLower(column.Name.String())] = column.Name.String()
			columns[column.Name.String()] = column
			columnType := column.Type
			originalType := strings.ToLower(columnType.Type)
			columnType.Type = storageType(originalType)
			if originalType == "bool" || originalType == "boolean" {
				length := 1
				columnType.Length = &length
			}
			switch columnType.Type {
			case "char", "binary", "bit":
				if columnType.Length == nil {
					length := 1
					columnType.Length = &length
				}
			case "decimal":
				if columnType.Length == nil {
					length := 10
					columnType.Length = &length
				}
				if columnType.Scale == nil {
					scale := 0
					columnType.Scale = &scale
				}
			case "datetime", "timestamp", "time":
				if columnType.Length != nil && *columnType.Length == 0 {
					columnType.Length = nil
				}
			}
			options := columnType.Options
			columnType.Charset.Name = strings.ToLower(unquoteOption(columnType.Charset.Name))
			options.Collate = strings.ToLower(unquoteOption(options.Collate))
			if columnType.Charset.Name == "" && options.Collate != "" {
				columnType.Charset.Name = strings.SplitN(options.Collate, "_", 2)[0]
			} else if columnType.Charset.Name == "" && options.Collate == "" {
				switch columnType.Type {
				case "char", "varchar", "tinytext", "text", "mediumtext", "longtext", "enum", "set":
					columnType.Charset.Name, options.Collate = charset, collation
				}
			}
			// A nullable column without an explicit default has DEFAULT NULL.
			// Parentheses around literal defaults do not change their value.
			switch options.Default.(type) {
			case *sqlparser.NullVal:
				if options.Null != nil && *options.Null {
					options.Default = nil
				}
			case *sqlparser.Literal:
				options.DefaultLiteral = true
			}
		}
		// Preserve identifier and string-literal spelling, but canonicalize
		// case-insensitive expression references and function/collation names.
		_ = sqlparser.Walk(func(node sqlparser.SQLNode) (bool, error) {
			switch expression := node.(type) {
			case *sqlparser.ColName:
				if name := columnNames[strings.ToLower(expression.Name.String())]; name != "" {
					expression.Name = sqlparser.NewIdentifierCI(name)
				}
			case *sqlparser.FuncExpr:
				expression.Name = sqlparser.NewIdentifierCI(strings.ToLower(expression.Name.String()))
			case *sqlparser.CurTimeFuncExpr:
				expression.Name = sqlparser.NewIdentifierCI(strings.ToLower(expression.Name.String()))
			case *sqlparser.CollateExpr:
				expression.Collation = strings.ToLower(expression.Collation)
			}
			return true, nil
		}, create)
		for _, index := range spec.Indexes {
			for _, part := range index.Columns {
				columnType := columns[part.Column.String()].Type
				if part.Length != nil && columnType.Length != nil && *part.Length == *columnType.Length {
					switch columnType.Type {
					case "char", "varchar", "binary", "varbinary":
						part.Length = nil // A full-length prefix indexes the entire column.
					}
				}
			}
		}
		// Without an explicit primary key, InnoDB clusters on the first UNIQUE
		// index with full, non-nullable, non-virtual columns. Preserve that
		// choice while ignoring the order of the remaining index definitions.
		clusteredKey := ""
		for _, index := range spec.Indexes {
			if index.Info.Type == sqlparser.IndexTypePrimary {
				clusteredKey = index.Info.Name.String()
				break
			}
			if clusteredKey != "" || index.Info.Type != sqlparser.IndexTypeUnique {
				continue
			}
			notNull := true
			for _, part := range index.Columns {
				options := columns[part.Column.String()].Type.Options
				if part.Length != nil || options.Null == nil || *options.Null || options.As != nil && options.Storage != sqlparser.StoredStorage {
					notNull = false
				}
			}
			if notNull {
				clusteredKey = index.Info.Name.String()
			}
		}
		slices.SortFunc(spec.Indexes, func(a, b *sqlparser.IndexDefinition) int {
			if a.Info.Name.String() == clusteredKey && b.Info.Name.String() != clusteredKey {
				return -1
			}
			if b.Info.Name.String() == clusteredKey && a.Info.Name.String() != clusteredKey {
				return 1
			}
			return strings.Compare(a.Info.Name.String(), b.Info.Name.String())
		})
		slices.SortFunc(spec.Constraints, func(a, b *sqlparser.ConstraintDefinition) int {
			return strings.Compare(sqlparser.String(a), sqlparser.String(b))
		})
		slices.SortFunc(spec.Options, func(a, b *sqlparser.TableOption) int { return strings.Compare(a.Name, b.Name) })
		canonical := schemaTable{name: table.Name, definitions: []schemaDefinition{}, clusteredKey: clusteredKey,
			graph: SchemaTable{Name: table.Name, Columns: []SchemaColumn{}, ForeignKeys: []SchemaForeignKey{}}}
		parts, order := []string{}, []string{}
		columnPositions := map[string]int{}
		for _, column := range spec.Columns {
			definition := sqlparser.String(column)
			canonical.definitions = append(canonical.definitions, schemaDefinition{kind: "column", name: column.Name.String(), sql: definition})
			parts = append(parts, definition)
			order = append(order, identifier(column.Name.String()))
			columnPositions[column.Name.String()] = len(canonical.graph.Columns)
			columnType := *column.Type
			options := columnType.Options
			attributes := map[string]string{}
			for name, expression := range map[string]sqlparser.Expr{"default": options.Default, "on update": options.OnUpdate, "generated": options.As} {
				if expression != nil {
					attributes[name] = sqlparser.String(expression)
				}
			}
			if options.As != nil {
				attributes["storage"] = "virtual"
				if options.Storage == sqlparser.StoredStorage {
					attributes["storage"] = "stored"
				}
			}
			if columnType.Charset.Name != "" {
				attributes["charset"] = columnType.Charset.Name
			}
			if options.Collate != "" {
				attributes["collation"] = options.Collate
			}
			if options.Autoincrement {
				attributes["auto increment"] = "yes"
			}
			if options.Comment != nil {
				attributes["comment"] = sqlparser.String(options.Comment)
			}
			if options.Invisible != nil && *options.Invisible {
				attributes["invisible"] = "yes"
			}
			columnType.Options = nil
			columnType.Charset = sqlparser.ColumnCharset{}
			canonical.graph.Columns = append(canonical.graph.Columns, SchemaColumn{Name: column.Name.String(), Type: sqlparser.String(&columnType), Nullable: options.Null != nil && *options.Null, Definition: definition, Attributes: attributes})
		}
		for _, index := range spec.Indexes {
			definition := sqlparser.String(index)
			canonical.definitions = append(canonical.definitions, schemaDefinition{kind: "index", name: index.Info.Name.String(), sql: definition})
			parts = append(parts, definition)
			if index.Info.Type == sqlparser.IndexTypePrimary {
				for _, part := range index.Columns {
					canonical.graph.Columns[columnPositions[part.Column.String()]].Primary = true
				}
			}
		}
		for _, constraint := range spec.Constraints {
			kind := "check"
			if foreign, ok := constraint.Details.(*sqlparser.ForeignKeyDefinition); ok {
				kind = "foreign key"
				for _, action := range []*sqlparser.ReferenceAction{&foreign.ReferenceDefinition.OnDelete, &foreign.ReferenceDefinition.OnUpdate} {
					if *action == sqlparser.Restrict || *action == sqlparser.NoAction {
						*action = sqlparser.DefaultAction
					}
				}
				reference := foreign.ReferenceDefinition
				relationship := SchemaForeignKey{Name: constraint.Name.String(), Columns: []string{}, ReferencedTable: reference.ReferencedTable.Name.String(), ReferencedColumns: []string{}, OnDelete: strings.ToUpper(sqlparser.String(reference.OnDelete)), OnUpdate: strings.ToUpper(sqlparser.String(reference.OnUpdate)), Definition: sqlparser.String(constraint)}
				for _, column := range foreign.Source {
					relationship.Columns = append(relationship.Columns, column.String())
					canonical.graph.Columns[columnPositions[column.String()]].Foreign = true
				}
				for _, column := range reference.ReferencedColumns {
					relationship.ReferencedColumns = append(relationship.ReferencedColumns, column.String())
				}
				for _, index := range spec.Indexes {
					if !index.Info.IsUnique() || len(index.Columns) == 0 {
						continue
					}
					unique := true
					for _, part := range index.Columns {
						if !slices.Contains(relationship.Columns, part.Column.String()) {
							unique = false
							break
						}
					}
					if unique {
						relationship.UniqueSource = true
						break
					}
				}
				canonical.graph.ForeignKeys = append(canonical.graph.ForeignKeys, relationship)
			}
			definition := sqlparser.String(constraint)
			canonical.definitions = append(canonical.definitions, schemaDefinition{kind: kind, name: constraint.Name.String(), sql: definition})
			parts = append(parts, definition)
		}
		options := sqlparser.String(spec.Options)
		canonical.definitions = append(canonical.definitions, schemaDefinition{kind: "options", name: "table options", sql: options})
		canonical.sql = "CREATE TABLE " + identifier(table.Name) + " (\n  " + strings.Join(parts, ",\n  ") + "\n) " + options + ";"
		canonical.columnOrder = strings.Join(order, ", ")
		tables = append(tables, canonical)
	}
	slices.SortFunc(tables, func(a, b schemaTable) int { return strings.Compare(a.name, b.name) })
	return tables, nil
}
