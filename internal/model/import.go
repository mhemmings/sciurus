package model

import (
	"fmt"
	"strings"

	"vitess.io/vitess/go/vt/sqlparser"
)

// ImportSQL converts supported CREATE TABLE statements into a structured model.
// Unsupported attributes are rejected, never retained in an opaque shadow schema
// which would diverge from the editable fields. Import assigns a deterministic
// starting grid; subsequent loads use the stored positions without re-layout.
func ImportSQL(source, name string) (*Document, error) {
	return importSQL(source, name, false)
}

// ImportSchemaSQL imports a schema for comparison, allowing an empty schema.
// It accepts the same supported statements and attributes as ImportSQL.
func ImportSchemaSQL(source, name string) (*Document, error) {
	return importSQL(source, name, true)
}

func importSQL(source, name string, allowEmpty bool) (*Document, error) {
	if len(source) > 16<<20 {
		return nil, fmt.Errorf("SQL exceeds the 16 MiB limit")
	}
	if strings.Contains(source, "/*!") {
		return nil, fmt.Errorf("conditional SQL comments are unsupported; import plain CREATE TABLE statements")
	}
	parser := sqlparser.NewTestParser()
	statements, err := parser.SplitStatementToPieces(source)
	if err != nil {
		return nil, fmt.Errorf("split SQL: %w", err)
	}
	document := New(name)
	for statementNumber, text := range statements {
		statement, parseErr := parser.ParseStrictDDL(text)
		if parseErr != nil {
			return nil, fmt.Errorf("statement %d: %w", statementNumber+1, parseErr)
		}
		if _, ok := statement.(*sqlparser.CommentOnly); ok {
			continue
		}
		if _, ok := statement.(*sqlparser.Set); ok {
			// Workbench's six session-wrapper statements do not define schema.
			// Restrict this exception so arbitrary SQL modes cannot change how
			// escaped strings and identifiers would be interpreted by MySQL.
			canonical := sqlparser.String(statement)
			allowed := false
			for _, wrapper := range []string{
				"SET @OLD_UNIQUE_CHECKS=@@UNIQUE_CHECKS, UNIQUE_CHECKS=0",
				"SET @OLD_FOREIGN_KEY_CHECKS=@@FOREIGN_KEY_CHECKS, FOREIGN_KEY_CHECKS=0",
				"SET @OLD_SQL_MODE=@@SQL_MODE, SQL_MODE='ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,NO_ZERO_IN_DATE,NO_ZERO_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION'",
				"SET SQL_MODE=@OLD_SQL_MODE", "SET FOREIGN_KEY_CHECKS=@OLD_FOREIGN_KEY_CHECKS", "SET UNIQUE_CHECKS=@OLD_UNIQUE_CHECKS",
				"SET FOREIGN_KEY_CHECKS=0", "SET FOREIGN_KEY_CHECKS=1",
			} {
				parsed, _ := parser.Parse(wrapper)
				if canonical == sqlparser.String(parsed) {
					allowed = true
					break
				}
			}
			if !allowed {
				return nil, fmt.Errorf("statement %d: only Workbench session wrapper SET statements are supported", statementNumber+1)
			}
			continue
		}
		create, ok := statement.(*sqlparser.CreateTable)
		if !ok || create.TableSpec == nil || create.Temp || create.OptLike != nil || create.Select != nil || !create.Table.Qualifier.IsEmpty() {
			return nil, fmt.Errorf("statement %d: only unqualified, non-temporary CREATE TABLE definitions are supported", statementNumber+1)
		}
		if create.TableSpec.PartitionOption != nil {
			return nil, fmt.Errorf("table %q: partitioning is unsupported", create.Table.Name.String())
		}
		table := Table{ID: create.Table.Name.String(), Name: create.Table.Name.String(), Engine: "InnoDB", Width: 300, Position: Point{X: float64(len(document.Tables)%5) * 380, Y: float64(len(document.Tables)/5) * 600}, Columns: []Column{}, Indexes: []Index{}, ForeignKeys: []ForeignKey{}}
		for _, definition := range create.TableSpec.Columns {
			columnType := *definition.Type
			options := columnType.Options
			if options == nil {
				options = &sqlparser.ColumnTypeOptions{}
			}
			if columnType.Charset.Binary || columnType.Zerofill || options.Reference != nil || options.Format != sqlparser.UnspecifiedFormat || options.EngineAttribute != nil || options.SecondaryEngineAttribute != nil || options.SRID != nil {
				return nil, fmt.Errorf("column %s.%s: unsupported BINARY marker, ZEROFILL, inline reference or engine/spatial attribute", table.Name, definition.Name.String())
			}
			column := Column{ID: definition.Name.String(), Name: definition.Name.String(), Nullable: options.Null == nil || *options.Null, AutoIncrement: options.Autoincrement, Charset: unquoteOption(columnType.Charset.Name), Collation: unquoteOption(options.Collate)}
			columnType.Options = nil
			columnType.Charset = sqlparser.ColumnCharset{}
			column.Type = sqlparser.String(&columnType)
			if options.Default != nil {
				value := sqlparser.String(options.Default)
				if !options.DefaultLiteral {
					value = "(" + value + ")"
				}
				column.Default = &value
			}
			if options.OnUpdate != nil {
				column.OnUpdate = sqlparser.String(options.OnUpdate)
			}
			if options.Comment != nil {
				column.Comment = options.Comment.Val
			}
			if options.As != nil {
				column.Generated = sqlparser.String(options.As)
				column.Stored = options.Storage == sqlparser.StoredStorage
			}
			if options.Invisible != nil {
				column.Invisible = *options.Invisible
			}
			// Inline keys are normalized into the same explicit index collection
			// as table-level keys so the UI has only one representation to edit.
			if options.KeyOpt != sqlparser.ColKeyNone {
				index := Index{ID: column.ID + "_key", Name: column.Name, Columns: []IndexColumn{{Column: column.ID}}}
				switch options.KeyOpt {
				case sqlparser.ColKeyPrimary, sqlparser.ColKey:
					index.ID = "PRIMARY"
					index.Name = "PRIMARY"
					index.Kind = "PRIMARY"
				case sqlparser.ColKeyUnique, sqlparser.ColKeyUniqueKey:
					index.Kind = "UNIQUE"
				case sqlparser.ColKeyFulltextKey:
					index.Kind = "FULLTEXT"
				case sqlparser.ColKeySpatialKey:
					index.Kind = "SPATIAL"
				default:
					return nil, fmt.Errorf("column %s.%s: unsupported inline key", table.Name, column.Name)
				}
				table.Indexes = append(table.Indexes, index)
			}
			table.Columns = append(table.Columns, column)
		}
		for _, definition := range create.TableSpec.Indexes {
			index := Index{ID: definition.Info.Name.String(), Name: definition.Info.Name.String(), Columns: []IndexColumn{}}
			switch definition.Info.Type {
			case sqlparser.IndexTypePrimary:
				index.ID = "PRIMARY"
				index.Name = "PRIMARY"
				index.Kind = "PRIMARY"
			case sqlparser.IndexTypeUnique:
				index.Kind = "UNIQUE"
			case sqlparser.IndexTypeDefault:
				index.Kind = "INDEX"
			case sqlparser.IndexTypeFullText:
				index.Kind = "FULLTEXT"
			case sqlparser.IndexTypeSpatial:
				index.Kind = "SPATIAL"
			default:
				return nil, fmt.Errorf("table %q: unsupported index type", table.Name)
			}
			if !definition.Info.ConstraintName.IsEmpty() {
				return nil, fmt.Errorf("table %q: separate UNIQUE constraint names are unsupported; name the index instead", table.Name)
			}
			for _, part := range definition.Columns {
				if part.Expression != nil {
					return nil, fmt.Errorf("table %q: functional indexes are unsupported; use a generated column", table.Name)
				}
				column := IndexColumn{Column: part.Column.String(), Descending: part.Direction == sqlparser.DescOrder}
				if part.Length != nil {
					column.Length = *part.Length
				}
				index.Columns = append(index.Columns, column)
			}
			if index.Name == "" {
				return nil, fmt.Errorf("table %q: indexes must have explicit names", table.Name)
			}
			for _, option := range definition.Options {
				switch strings.ToLower(option.Name) {
				case "visible":
				case "invisible":
					index.Invisible = true
				case "comment":
					if option.Value != nil {
						index.Comment = option.Value.Val
					}
				case "using":
					if !strings.EqualFold(option.String, "btree") {
						return nil, fmt.Errorf("index %s.%s: only BTREE is supported", table.Name, index.Name)
					}
				default:
					return nil, fmt.Errorf("index %s.%s: unsupported option %q", table.Name, index.Name, option.Name)
				}
			}
			table.Indexes = append(table.Indexes, index)
		}
		for _, definition := range create.TableSpec.Constraints {
			if definition.Name.IsEmpty() {
				return nil, fmt.Errorf("table %q: constraints must have explicit names", table.Name)
			}
			switch details := definition.Details.(type) {
			case *sqlparser.ForeignKeyDefinition:
				reference := details.ReferenceDefinition
				if reference.Match != sqlparser.DefaultMatch || !reference.ReferencedTable.Qualifier.IsEmpty() || !details.IndexName.IsEmpty() {
					return nil, fmt.Errorf("foreign key %q: MATCH, qualified references and inline index names are unsupported", definition.Name.String())
				}
				foreign := ForeignKey{ID: definition.Name.String(), Name: definition.Name.String(), ReferencedTable: reference.ReferencedTable.Name.String(), Columns: []string{}, ReferencedColumns: []string{}}
				for _, column := range details.Source {
					foreign.Columns = append(foreign.Columns, column.String())
				}
				for _, column := range reference.ReferencedColumns {
					foreign.ReferencedColumns = append(foreign.ReferencedColumns, column.String())
				}
				actions := map[sqlparser.ReferenceAction]string{sqlparser.DefaultAction: "", sqlparser.Restrict: "RESTRICT", sqlparser.Cascade: "CASCADE", sqlparser.NoAction: "NO ACTION", sqlparser.SetNull: "SET NULL"}
				var supported bool
				foreign.OnDelete, supported = actions[reference.OnDelete]
				if !supported {
					return nil, fmt.Errorf("foreign key %q: unsupported delete action", foreign.Name)
				}
				foreign.OnUpdate, supported = actions[reference.OnUpdate]
				if !supported {
					return nil, fmt.Errorf("foreign key %q: unsupported update action", foreign.Name)
				}
				table.ForeignKeys = append(table.ForeignKeys, foreign)
			case *sqlparser.CheckConstraintDefinition:
				enforced := details.Enforced
				table.Checks = append(table.Checks, Check{Name: definition.Name.String(), Expression: sqlparser.String(details.Expr), Enforced: &enforced})
			default:
				return nil, fmt.Errorf("table %q: unsupported constraint", table.Name)
			}
		}
		seenOptions := map[string]bool{}
		for _, option := range create.TableSpec.Options {
			key := strings.ToLower(option.Name)
			if seenOptions[key] {
				return nil, fmt.Errorf("table %q: duplicate option %q", table.Name, key)
			}
			seenOptions[key] = true
			value := option.String
			if option.Value != nil {
				value = option.Value.Val
			}
			switch key {
			case "engine":
				table.Engine = value
			case "charset", "character set":
				table.Charset = unquoteOption(value)
			case "collate":
				table.Collation = unquoteOption(value)
			case "comment":
				table.Comment = value
			default:
				return nil, fmt.Errorf("table %q: unsupported table option %q", table.Name, option.Name)
			}
		}
		document.Tables = append(document.Tables, table)
	}
	if len(document.Tables) == 0 && !allowEmpty {
		return nil, fmt.Errorf("SQL contains no CREATE TABLE definitions")
	}
	// MySQL column identifiers are case-insensitive. Resolve their SQL spelling
	// to the exact saved ID, including references in indexes and foreign keys.
	columnIDs := map[string]map[string]string{}
	for _, table := range document.Tables {
		columnIDs[table.ID] = map[string]string{}
		for _, column := range table.Columns {
			columnIDs[table.ID][strings.ToLower(column.Name)] = column.ID
		}
	}
	for ti := range document.Tables {
		table := &document.Tables[ti]
		for ii := range table.Indexes {
			for ci := range table.Indexes[ii].Columns {
				part := &table.Indexes[ii].Columns[ci]
				if resolved := columnIDs[table.ID][strings.ToLower(part.Column)]; resolved != "" {
					part.Column = resolved
				}
			}
		}
		for fi := range table.ForeignKeys {
			key := &table.ForeignKeys[fi]
			for ci, id := range key.Columns {
				if resolved := columnIDs[table.ID][strings.ToLower(id)]; resolved != "" {
					key.Columns[ci] = resolved
				}
			}
			for ci, id := range key.ReferencedColumns {
				if resolved := columnIDs[key.ReferencedTable][strings.ToLower(id)]; resolved != "" {
					key.ReferencedColumns[ci] = resolved
				}
			}
		}
		// Normalize implicit PRIMARY KEY nullability once, after every key's
		// column spelling has been resolved to its saved ID.
		for _, index := range table.Indexes {
			if index.Kind == "PRIMARY" {
				for _, part := range index.Columns {
					for ci := range table.Columns {
						if table.Columns[ci].ID == part.Column {
							table.Columns[ci].Nullable = false
						}
					}
				}
			}
		}
	}
	err = Validate(document)
	if err != nil {
		return nil, err
	}
	return document, nil
}

// unquoteOption handles the SQL parser's encoded charset/collation names. These
// names are restricted to ASCII word characters during model validation.
func unquoteOption(value string) string { return strings.Trim(value, "'`\"") }
