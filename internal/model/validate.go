package model

import (
	"fmt"
	"math"
	"reflect"
	"regexp"
	"strings"
	"unicode/utf8"

	"vitess.io/vitess/go/vt/sqlparser"
)

// mysqlWord excludes SQL punctuation from charset and collation attributes.
var mysqlWord = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

// groupColor limits layer colors to portable CSS hex values.
var groupColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// Validate rejects inconsistent links, ambiguous identifiers, unsupported MySQL
// features and invalid layout before a document can be saved or used for SQL.
func Validate(document *Document) error {
	if document == nil {
		return fmt.Errorf("model must be an object")
	}
	if document.Version != 1 {
		return fmt.Errorf("unsupported model version %d (expected 1)", document.Version)
	}
	if strings.TrimSpace(document.Name) == "" {
		return fmt.Errorf("model name is required")
	}
	if document.Tables == nil || document.Groups == nil {
		return fmt.Errorf("tables and groups must be arrays (use [] when empty)")
	}
	if len(document.Tables) > 4096 {
		return fmt.Errorf("model exceeds 4096 tables")
	}
	if !coordinate(document.Viewport.X) || !coordinate(document.Viewport.Y) || !finite(document.Viewport.Zoom) || document.Viewport.Zoom < 0.02 || document.Viewport.Zoom > 4 {
		return fmt.Errorf("viewport must have finite coordinates and zoom between 0.02 and 4")
	}
	for _, value := range []string{document.Charset, document.Collation} {
		if value != "" && !mysqlWord.MatchString(value) {
			return fmt.Errorf("invalid schema charset or collation %q", value)
		}
	}
	groups := map[string]bool{}
	for _, group := range document.Groups {
		if !validID(group.ID) || groups[group.ID] {
			return fmt.Errorf("invalid or duplicate group ID %q", group.ID)
		}
		groups[group.ID] = true
		if strings.TrimSpace(group.Name) == "" || !groupColor.MatchString(group.Color) {
			return fmt.Errorf("group %q requires a name and #RRGGBB color", group.ID)
		}
		if !coordinate(group.Position.X) || !coordinate(group.Position.Y) || !size(group.Width, 80) || !size(group.Height, 60) {
			return fmt.Errorf("group %q has invalid bounds", group.ID)
		}
	}
	tables, tableNames := map[string]*Table{}, map[string]bool{}
	columnTypes := map[string]map[string]*sqlparser.ColumnType{}
	parser := sqlparser.NewTestParser()
	constraintNames := map[string]bool{}
	foreignIDs := map[string]bool{}
	for i := range document.Tables {
		table := &document.Tables[i]
		if !validID(table.ID) || tables[table.ID] != nil {
			return fmt.Errorf("invalid or duplicate table ID %q", table.ID)
		}
		tables[table.ID] = table
		columnTypes[table.ID] = map[string]*sqlparser.ColumnType{}
		if !validName(table.Name) || tableNames[strings.ToLower(table.Name)] {
			return fmt.Errorf("invalid or duplicate table name %q", table.Name)
		}
		tableNames[strings.ToLower(table.Name)] = true
		if table.Engine != "" && !strings.EqualFold(table.Engine, "InnoDB") {
			return fmt.Errorf("table %q: only InnoDB is supported", table.Name)
		}
		if table.GroupID != "" && !groups[table.GroupID] {
			return fmt.Errorf("table %q references missing group %q", table.Name, table.GroupID)
		}
		if !coordinate(table.Position.X) || !coordinate(table.Position.Y) || !size(table.Width, 80) {
			return fmt.Errorf("table %q has invalid position or width", table.Name)
		}
		for _, value := range []string{table.Charset, table.Collation} {
			if value != "" && !mysqlWord.MatchString(value) {
				return fmt.Errorf("table %q has invalid charset or collation", table.Name)
			}
		}
		if len(table.Columns) == 0 || len(table.Columns) > 1017 {
			return fmt.Errorf("table %q must have 1–1017 columns", table.Name)
		}
		if table.Indexes == nil || table.ForeignKeys == nil {
			return fmt.Errorf("table %q: indexes and foreignKeys must be arrays (use [] when empty)", table.Name)
		}
		columns, columnNames := map[string]*Column{}, map[string]bool{}
		autoIncrement := 0
		for j := range table.Columns {
			column := &table.Columns[j]
			if !validID(column.ID) || columns[column.ID] != nil {
				return fmt.Errorf("table %q: invalid or duplicate column ID %q", table.Name, column.ID)
			}
			columns[column.ID] = column
			if !validName(column.Name) || columnNames[strings.ToLower(column.Name)] {
				return fmt.Errorf("table %q: invalid or duplicate column name %q", table.Name, column.Name)
			}
			columnNames[strings.ToLower(column.Name)] = true
			fragments := []string{column.Type, column.Generated, column.OnUpdate}
			if column.Default != nil {
				fragments = append(fragments, *column.Default)
			}
			for _, fragment := range fragments {
				if strings.Contains(fragment, "/*!") {
					return fmt.Errorf("column %s.%s: conditional SQL comments are unsupported", table.Name, column.Name)
				}
			}
			for _, value := range []string{column.Charset, column.Collation} {
				if value != "" && !mysqlWord.MatchString(value) {
					return fmt.Errorf("column %s.%s has invalid charset or collation", table.Name, column.Name)
				}
			}
			// A type field may contain only a type, never a second definition or
			// column attributes which would compete with the structured fields.
			statement, err := parser.ParseStrictDDL("CREATE TABLE `_type` (`_column` " + column.Type + ")")
			if err != nil {
				return fmt.Errorf("column %s.%s type: %w", table.Name, column.Name, err)
			}
			create, ok := statement.(*sqlparser.CreateTable)
			if !ok || create.TableSpec == nil || len(create.TableSpec.Columns) != 1 || len(create.TableSpec.Indexes) != 0 || len(create.TableSpec.Constraints) != 0 || len(create.TableSpec.Options) != 0 || create.TableSpec.PartitionOption != nil {
				return fmt.Errorf("column %s.%s: type must contain only a MySQL data type", table.Name, column.Name)
			}
			parsedType := create.TableSpec.Columns[0].Type
			columnTypes[table.ID][column.ID] = parsedType
			if parsedType.Options != nil && !reflect.DeepEqual(*parsedType.Options, sqlparser.ColumnTypeOptions{}) || parsedType.Charset != (sqlparser.ColumnCharset{}) || parsedType.Zerofill {
				return fmt.Errorf("column %s.%s: use structured attributes; ZEROFILL is unsupported", table.Name, column.Name)
			}
			switch strings.ToLower(parsedType.Type) {
			case "varchar", "varbinary":
				if parsedType.Length == nil || *parsedType.Length < 1 || *parsedType.Length > 65535 {
					return fmt.Errorf("column %s.%s: %s requires a length between 1 and 65535", table.Name, column.Name, parsedType.Type)
				}
			case "char", "binary":
				if parsedType.Length != nil && (*parsedType.Length < 0 || *parsedType.Length > 255) {
					return fmt.Errorf("column %s.%s: %s length must be between 0 and 255", table.Name, column.Name, parsedType.Type)
				}
			case "datetime", "timestamp", "time":
				if parsedType.Length != nil && (*parsedType.Length < 0 || *parsedType.Length > 6) {
					return fmt.Errorf("column %s.%s: fractional precision must be between 0 and 6", table.Name, column.Name)
				}
			case "decimal", "numeric":
				if parsedType.Length != nil && (*parsedType.Length < 1 || *parsedType.Length > 65) || parsedType.Scale != nil && (*parsedType.Scale < 0 || *parsedType.Scale > 30 || parsedType.Length != nil && *parsedType.Scale > *parsedType.Length) {
					return fmt.Errorf("column %s.%s: invalid DECIMAL precision or scale", table.Name, column.Name)
				}
			case "bit":
				if parsedType.Length != nil && (*parsedType.Length < 1 || *parsedType.Length > 64) {
					return fmt.Errorf("column %s.%s: BIT length must be between 1 and 64", table.Name, column.Name)
				}
			}
			if column.Generated != "" && (column.Default != nil || column.AutoIncrement || column.OnUpdate != "") {
				return fmt.Errorf("generated column %s.%s cannot have a default, AUTO_INCREMENT or ON UPDATE", table.Name, column.Name)
			}
			if column.Stored && column.Generated == "" {
				return fmt.Errorf("column %s.%s: STORED needs a generated expression", table.Name, column.Name)
			}
			if column.AutoIncrement {
				autoIncrement++
				switch strings.ToLower(parsedType.Type) {
				case "tinyint", "smallint", "mediumint", "int", "integer", "bigint":
				default:
					return fmt.Errorf("column %s.%s: AUTO_INCREMENT requires an integer type", table.Name, column.Name)
				}
				if column.Nullable || column.Default != nil {
					return fmt.Errorf("column %s.%s: AUTO_INCREMENT must be NOT NULL without an explicit default", table.Name, column.Name)
				}
			}
			if column.Default != nil && strings.TrimSpace(*column.Default) == "" {
				return fmt.Errorf("column %s.%s: default cannot be blank; use '' for an empty string", table.Name, column.Name)
			}
		}
		if autoIncrement > 1 {
			return fmt.Errorf("table %q has more than one AUTO_INCREMENT column", table.Name)
		}
		indexIDs, indexNames := map[string]bool{}, map[string]bool{}
		primaryCount := 0
		for _, index := range table.Indexes {
			if !validID(index.ID) || indexIDs[index.ID] {
				return fmt.Errorf("table %q: invalid or duplicate index ID %q", table.Name, index.ID)
			}
			indexIDs[index.ID] = true
			if !validName(index.Name) || indexNames[strings.ToLower(index.Name)] {
				return fmt.Errorf("table %q: invalid or duplicate index name %q", table.Name, index.Name)
			}
			indexNames[strings.ToLower(index.Name)] = true
			switch index.Kind {
			case "PRIMARY":
				primaryCount++
				if index.Name != "PRIMARY" || index.Invisible {
					return fmt.Errorf("table %q: PRIMARY key must be named PRIMARY and visible", table.Name)
				}
			case "INDEX", "UNIQUE", "FULLTEXT", "SPATIAL":
				if strings.EqualFold(index.Name, "PRIMARY") {
					return fmt.Errorf("table %q: PRIMARY is reserved for the primary key", table.Name)
				}
			default:
				return fmt.Errorf("table %q: unsupported index kind %q", table.Name, index.Kind)
			}
			if len(index.Columns) == 0 || len(index.Columns) > 16 {
				return fmt.Errorf("index %s.%s must contain 1–16 columns", table.Name, index.Name)
			}
			seen := map[string]bool{}
			for _, part := range index.Columns {
				column := columns[part.Column]
				if column == nil || seen[part.Column] {
					return fmt.Errorf("index %s.%s has missing or duplicate column %q", table.Name, index.Name, part.Column)
				}
				seen[part.Column] = true
				if part.Length < 0 || part.Length > 65535 {
					return fmt.Errorf("index %s.%s has invalid prefix length", table.Name, index.Name)
				}
				if index.Kind == "PRIMARY" && column.Nullable {
					return fmt.Errorf("primary column %s.%s must be NOT NULL", table.Name, column.Name)
				}
			}
		}
		if primaryCount > 1 {
			return fmt.Errorf("table %q has multiple PRIMARY keys", table.Name)
		}
		for _, column := range table.Columns {
			if column.AutoIncrement {
				indexed := false
				for _, index := range table.Indexes {
					if len(index.Columns) > 0 && index.Columns[0].Column == column.ID && index.Columns[0].Length == 0 && !index.Invisible && (index.Kind == "PRIMARY" || index.Kind == "UNIQUE" || index.Kind == "INDEX") {
						indexed = true
					}
				}
				if !indexed {
					return fmt.Errorf("AUTO_INCREMENT column %s.%s must lead a visible index", table.Name, column.Name)
				}
			}
		}
		for _, foreign := range table.ForeignKeys {
			if !validID(foreign.ID) || foreignIDs[foreign.ID] {
				return fmt.Errorf("table %q: invalid or duplicate relationship ID %q", table.Name, foreign.ID)
			}
			foreignIDs[foreign.ID] = true
			if !validName(foreign.Name) || constraintNames["fk:"+strings.ToLower(foreign.Name)] {
				return fmt.Errorf("invalid or duplicate foreign key name %q", foreign.Name)
			}
			constraintNames["fk:"+strings.ToLower(foreign.Name)] = true
			if len(foreign.Columns) == 0 || len(foreign.Columns) != len(foreign.ReferencedColumns) {
				return fmt.Errorf("foreign key %q requires equally sized nonempty column lists", foreign.Name)
			}
			for _, action := range []string{foreign.OnDelete, foreign.OnUpdate} {
				switch action {
				case "", "NO ACTION", "RESTRICT", "CASCADE", "SET NULL":
				default:
					return fmt.Errorf("foreign key %q has unsupported action %q", foreign.Name, action)
				}
			}
			seen := map[string]bool{}
			for _, id := range foreign.Columns {
				column := columns[id]
				if column == nil || seen[id] {
					return fmt.Errorf("foreign key %q has missing or duplicate source column %q", foreign.Name, id)
				}
				seen[id] = true
				if (foreign.OnDelete == "SET NULL" || foreign.OnUpdate == "SET NULL") && !column.Nullable {
					return fmt.Errorf("foreign key %q: SET NULL requires nullable source columns", foreign.Name)
				}
			}
			for _, point := range foreign.Points {
				if !coordinate(point.X) || !coordinate(point.Y) {
					return fmt.Errorf("foreign key %q has invalid route coordinates", foreign.Name)
				}
			}
		}
		for _, check := range table.Checks {
			if strings.Contains(check.Expression, "/*!") {
				return fmt.Errorf("check %q: conditional SQL comments are unsupported", check.Name)
			}
			if !validName(check.Name) || constraintNames["check:"+strings.ToLower(check.Name)] || strings.TrimSpace(check.Expression) == "" {
				return fmt.Errorf("invalid or duplicate CHECK constraint %q", check.Name)
			}
			constraintNames["check:"+strings.ToLower(check.Name)] = true
		}
	}
	// Resolve cross-table links only after every table has been indexed, which
	// permits self-references and cycles independently of document table order.
	for _, table := range document.Tables {
		columnNames := map[string]bool{}
		for _, column := range table.Columns {
			columnNames[strings.ToLower(column.Name)] = true
		}
		for _, column := range table.Columns {
			for _, expression := range []string{column.Generated, column.OnUpdate} {
				if expression != "" {
					err := validateExpression(parser, expression, columnNames)
					if err != nil {
						return fmt.Errorf("column %s.%s: %w", table.Name, column.Name, err)
					}
				}
			}
			if column.Default != nil {
				err := validateExpression(parser, *column.Default, nil)
				if err != nil {
					return fmt.Errorf("column %s.%s default: %w", table.Name, column.Name, err)
				}
			}
		}
		for _, check := range table.Checks {
			err := validateExpression(parser, check.Expression, columnNames)
			if err != nil {
				return fmt.Errorf("check %q: %w", check.Name, err)
			}
		}
		for _, foreign := range table.ForeignKeys {
			target := tables[foreign.ReferencedTable]
			if target == nil {
				return fmt.Errorf("foreign key %q references missing table %q", foreign.Name, foreign.ReferencedTable)
			}
			seen := map[string]bool{}
			for _, id := range foreign.ReferencedColumns {
				found := false
				for _, column := range target.Columns {
					if column.ID == id {
						found = true
						break
					}
				}
				if !found || seen[id] {
					return fmt.Errorf("foreign key %q has missing or duplicate target column %q", foreign.Name, id)
				}
				seen[id] = true
			}
			// MySQL 8.4's standard referenced keys are full PRIMARY/UNIQUE keys,
			// in the same order, without prefixes or invisible indexes.
			indexed := false
			for _, index := range target.Indexes {
				if index.Invisible || (index.Kind != "PRIMARY" && index.Kind != "UNIQUE") || len(index.Columns) != len(foreign.ReferencedColumns) {
					continue
				}
				match := true
				for i, part := range index.Columns {
					if part.Column != foreign.ReferencedColumns[i] || part.Length != 0 {
						match = false
						break
					}
				}
				if match {
					indexed = true
					break
				}
			}
			if !indexed {
				return fmt.Errorf("foreign key %q: referenced columns must match a complete visible PRIMARY or UNIQUE key", foreign.Name)
			}
			for i, sourceID := range foreign.Columns {
				sourceType, targetType := columnTypes[table.ID][sourceID], columnTypes[target.ID][foreign.ReferencedColumns[i]]
				if storageType(sourceType.Type) != storageType(targetType.Type) || sourceType.Unsigned != targetType.Unsigned {
					return fmt.Errorf("foreign key %q: source and referenced column types/signs must match", foreign.Name)
				}
				if storageType(sourceType.Type) == "decimal" && (!sameInt(sourceType.Length, targetType.Length) || !sameInt(sourceType.Scale, targetType.Scale)) {
					return fmt.Errorf("foreign key %q: DECIMAL precision and scale must match", foreign.Name)
				}
				var sourceColumn, targetColumn *Column
				for j := range table.Columns {
					if table.Columns[j].ID == sourceID {
						sourceColumn = &table.Columns[j]
						break
					}
				}
				for j := range target.Columns {
					if target.Columns[j].ID == foreign.ReferencedColumns[i] {
						targetColumn = &target.Columns[j]
						break
					}
				}
				textual := false
				switch storageType(sourceType.Type) {
				case "char", "varchar", "tinytext", "text", "mediumtext", "longtext", "enum", "set":
					textual = true
				}
				for _, property := range []string{"charset", "collation"} {
					if !textual {
						break
					}
					sourceValue, targetValue := effectiveTextOption(document, &table, sourceColumn, property), effectiveTextOption(document, target, targetColumn, property)
					if sourceValue != "" && targetValue != "" && !strings.EqualFold(sourceValue, targetValue) {
						return fmt.Errorf("foreign key %q: explicit column %s values must match", foreign.Name, property)
					}
				}
			}
		}
		// Parse the complete statement and compare shape to catch injection into
		// expression fields without attempting to implement a SQL expression parser.
		statement, err := parser.ParseStrictDDL(tableSQL(document, &table))
		if err != nil {
			return fmt.Errorf("table %q SQL: %w", table.Name, err)
		}
		create, ok := statement.(*sqlparser.CreateTable)
		if !ok || create.TableSpec == nil || len(create.TableSpec.Columns) != len(table.Columns) || len(create.TableSpec.Indexes) != len(table.Indexes) || len(create.TableSpec.Constraints) != len(table.ForeignKeys)+len(table.Checks) {
			return fmt.Errorf("table %q: expressions must not introduce SQL definitions", table.Name)
		}
		for i, column := range table.Columns {
			_, nullDefault := create.TableSpec.Columns[i].Type.Options.Default.(*sqlparser.NullVal)
			if !column.Nullable && nullDefault {
				return fmt.Errorf("column %s.%s: NOT NULL columns cannot have DEFAULT NULL", table.Name, column.Name)
			}
		}
	}
	return nil
}

// storageType normalizes MySQL's common type aliases for FK compatibility.
func storageType(value string) string {
	switch strings.ToLower(value) {
	case "integer":
		return "int"
	case "numeric", "dec", "fixed":
		return "decimal"
	case "bool", "boolean":
		return "tinyint"
	default:
		return strings.ToLower(value)
	}
}

// sameInt compares optional precision metadata without pointer identity.
func sameInt(a, b *int) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }

// effectiveTextOption resolves explicit defaults only; an unknown server default
// stays unknown, rather than assuming the server uses a particular collation.
func effectiveTextOption(document *Document, table *Table, column *Column, property string) string {
	if property == "charset" {
		if column.Charset != "" {
			return column.Charset
		}
		if column.Collation != "" {
			return strings.SplitN(column.Collation, "_", 2)[0]
		}
		if table.Charset != "" {
			return table.Charset
		}
		if table.Collation != "" {
			return strings.SplitN(table.Collation, "_", 2)[0]
		}
		if document.Charset != "" {
			return document.Charset
		}
		if document.Collation != "" {
			return strings.SplitN(document.Collation, "_", 2)[0]
		}
		return ""
	}
	if column.Collation != "" {
		return column.Collation
	}
	if column.Charset != "" {
		return ""
	}
	if table.Collation != "" {
		return table.Collation
	}
	if table.Charset != "" {
		return ""
	}
	return document.Collation
}

// validateExpression uses the full SELECT grammar, avoiding ParseExpr's partial
// first-expression convenience API. Column names in generated/check expressions
// are checked so a rename cannot silently leave an invalid expression behind.
func validateExpression(parser *sqlparser.Parser, expression string, columns map[string]bool) error {
	statement, err := parser.Parse("SELECT (" + expression + ")")
	if err != nil {
		return fmt.Errorf("invalid SQL expression: %w", err)
	}
	selection, ok := statement.(*sqlparser.Select)
	if !ok || len(selection.SelectExprs.Exprs) != 1 {
		return fmt.Errorf("expected exactly one SQL expression")
	}
	return sqlparser.Walk(func(node sqlparser.SQLNode) (bool, error) {
		switch value := node.(type) {
		case *sqlparser.Subquery, *sqlparser.Argument, sqlparser.ListArg:
			return false, fmt.Errorf("subqueries and bind parameters are unsupported in schema expressions")
		case *sqlparser.ColName:
			if !value.Qualifier.IsEmpty() || !columns[strings.ToLower(value.Name.String())] {
				return false, fmt.Errorf("expression references unknown or disallowed column %q", value.Name.String())
			}
		}
		return true, nil
	}, selection.SelectExprs.Exprs[0])
}

// finite excludes values which cannot be represented portably in JSON or CSS.
func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

// coordinate bounds canvas arithmetic while permitting negative placements.
func coordinate(value float64) bool { return finite(value) && math.Abs(value) <= 1e7 }

// size requires a finite, usable rectangle dimension.
func size(value, minimum float64) bool { return finite(value) && value >= minimum && value <= 1e7 }

// validID keeps opaque references readable and rejects empty or control-only IDs.
func validID(value string) bool {
	return strings.TrimSpace(value) != "" && len(value) <= 200 && !strings.ContainsAny(value, "\x00\r\n\t")
}

// validName accepts quoted MySQL identifiers up to the server's 64-character limit.
func validName(value string) bool {
	return strings.TrimSpace(value) != "" && utf8.ValidString(value) && utf8.RuneCountInString(value) <= 64 && !strings.ContainsAny(value, "\x00\r\n\t") && !strings.HasSuffix(value, " ")
}
