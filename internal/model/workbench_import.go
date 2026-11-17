package model

import (
	"fmt"
	"strconv"
	"strings"

	"vitess.io/vitess/go/vt/sqlparser"
)

// ImportWorkbench reads a single MySQL schema and its optional diagram. Catalog
// definitions pass through the SQL importer so both formats share the same
// normalization and supported-schema checks.
func ImportWorkbench(data []byte, name string) (*Document, error) {
	archive, err := readWorkbench(data)
	if err != nil {
		return nil, err
	}
	var schema *workbenchNode
	hasDiagram := false
	for _, node := range archive.ordered {
		if node.StructName == "workbench.physical.Diagram" {
			hasDiagram = true
		}
		if node.StructName != "db.mysql.Schema" || node.excluded() {
			continue
		}
		if schema != nil {
			return nil, fmt.Errorf("Workbench import requires exactly one MySQL schema")
		}
		schema = node
	}
	if schema == nil {
		return nil, fmt.Errorf("Workbench archive has no MySQL schema")
	}
	if err := schema.unsupported("views", "routines", "routineGroups", "events", "sequences", "synonyms", "structuredTypes"); err != nil {
		return nil, err
	}
	tables, err := archive.members(schema, "tables", "db.mysql.Table")
	if err != nil {
		return nil, err
	}
	physical := map[string]*workbenchNode{}
	columns := map[string]map[string]string{}
	for _, table := range tables {
		if table.excluded() {
			continue
		}
		physical[table.ID] = table
		columns[table.ID] = map[string]string{}
		members, err := archive.members(table, "columns", "db.mysql.Column")
		if err != nil {
			return nil, err
		}
		for _, column := range members {
			columns[table.ID][column.ID] = column.field("name")
		}
	}
	var sql strings.Builder
	for _, table := range tables {
		if table.excluded() {
			continue
		}
		if err := table.unsupported("isTemporary", "temporaryScope", "isStub", "isSystem", "triggers", "partitionType", "partitionExpression", "partitionCount", "partitionKeyAlgorithm", "partitionDefinitions", "subpartitionType", "subpartitionExpression", "subpartitionCount", "subpartitionKeyAlgorithm", "connection", "connectionString", "nextAutoInc", "password", "delayKeyWrite", "mergeUnion", "mergeInsert", "tableDataDir", "tableIndexDir", "packKeys", "raidType", "raidChunks", "raidChunkSize", "checksum", "rowFormat", "keyBlockSize", "avgRowLength", "minRows", "maxRows", "statsAutoRecalc", "statsPersistent", "statsSamplePages", "tableSpace"); err != nil {
			return nil, err
		}
		definitions := []string{}
		members, err := archive.members(table, "columns", "db.mysql.Column")
		if err != nil {
			return nil, err
		}
		for _, column := range members {
			definition, err := column.workbenchColumnSQL()
			if err != nil {
				return nil, err
			}
			definitions = append(definitions, definition)
			checks, err := archive.workbenchChecks(column)
			if err != nil {
				return nil, err
			}
			definitions = append(definitions, checks...)
		}
		foreignKeys, err := archive.members(table, "foreignKeys", "db.mysql.ForeignKey")
		if err != nil {
			return nil, err
		}
		modelOnlyIndexes, physicalIndexes := map[string]bool{}, map[string]bool{}
		for _, foreign := range foreignKeys {
			if foreign.excluded() {
				modelOnlyIndexes[foreign.field("index")] = true
			} else {
				physicalIndexes[foreign.field("index")] = true
			}
		}
		indices, err := archive.members(table, "indices", "db.mysql.Index")
		if err != nil {
			return nil, err
		}
		for _, index := range indices {
			if index.excluded() {
				continue
			}
			// Workbench omits an index maintained exclusively for model-only
			// relationships, but retains shared indexes and primary keys.
			if index.field("isPrimary") != "1" && !strings.EqualFold(index.field("indexType"), "PRIMARY") && modelOnlyIndexes[index.ID] && !physicalIndexes[index.ID] {
				continue
			}
			if err := index.unsupported("keyBlockSize", "withParser", "deferability"); err != nil {
				return nil, err
			}
			kind := strings.ToUpper(index.field("indexType"))
			if index.field("isPrimary") == "1" {
				kind = "PRIMARY"
			} else if kind == "" {
				kind = "INDEX"
				if index.field("unique") == "1" {
					kind = "UNIQUE"
				}
			}
			definition := ""
			switch kind {
			case "PRIMARY":
				definition = "PRIMARY KEY"
			case "INDEX":
				definition = "INDEX " + identifier(index.field("name"))
			case "UNIQUE", "FULLTEXT", "SPATIAL":
				definition = kind + " INDEX " + identifier(index.field("name"))
			default:
				return nil, fmt.Errorf("Workbench index %q has unsupported type %q", index.field("name"), kind)
			}
			indexKind := strings.ToUpper(index.field("indexKind"))
			if indexKind != "" && indexKind != "BTREE" && !(indexKind == "RTREE" && kind == "SPATIAL") {
				return nil, fmt.Errorf("Workbench index %q has unsupported index kind %q", index.field("name"), indexKind)
			}
			parts, err := archive.members(index, "columns", "db.mysql.IndexColumn")
			if err != nil {
				return nil, err
			}
			keys := []string{}
			for _, part := range parts {
				if err := part.unsupported("expression"); err != nil {
					return nil, err
				}
				column, exists := columns[table.ID][part.field("referencedColumn")]
				if !exists {
					return nil, fmt.Errorf("Workbench index %q references a missing column", index.field("name"))
				}
				key := identifier(column)
				length := part.field("columnLength")
				if length != "" && length != "0" {
					value, err := strconv.Atoi(length)
					if err != nil || value < 0 {
						return nil, fmt.Errorf("Workbench index %q has invalid column prefix %q", index.field("name"), length)
					}
					key += "(" + length + ")"
				}
				if part.field("descend") == "1" {
					key += " DESC"
				}
				keys = append(keys, key)
			}
			definition += " (" + strings.Join(keys, ", ") + ")"
			if index.field("visible") == "0" {
				definition += " INVISIBLE"
			}
			if comment := index.field("comment"); comment != "" {
				definition += " COMMENT " + literal(comment)
			}
			definitions = append(definitions, definition)
		}
		for _, foreign := range foreignKeys {
			// Model-only relationships are drawings, not database constraints.
			if foreign.excluded() {
				continue
			}
			if err := foreign.unsupported("deferability"); err != nil {
				return nil, err
			}
			target := physical[foreign.field("referencedTable")]
			if target == nil {
				if archived := archive.objects[foreign.field("referencedTable")]; archived != nil && archived.excluded() {
					continue
				}
				return nil, fmt.Errorf("Workbench foreign key %q references a missing physical table", foreign.field("name"))
			}
			sourceColumns, err := archive.columnNames(foreign, "columns", columns[table.ID])
			if err != nil {
				return nil, err
			}
			targetColumns, err := archive.columnNames(foreign, "referencedColumns", columns[target.ID])
			if err != nil {
				return nil, err
			}
			definition := "CONSTRAINT " + identifier(foreign.field("name")) + " FOREIGN KEY (" + strings.Join(sourceColumns, ", ") + ") REFERENCES " + identifier(target.field("name")) + " (" + strings.Join(targetColumns, ", ") + ")"
			for _, action := range []struct{ key, sql string }{{"deleteRule", " ON DELETE "}, {"updateRule", " ON UPDATE "}} {
				if rule := foreign.field(action.key); rule != "" {
					switch strings.ToUpper(rule) {
					case "NO ACTION", "RESTRICT", "CASCADE", "SET NULL":
						definition += action.sql + strings.ToUpper(rule)
					default:
						return nil, fmt.Errorf("Workbench foreign key %q has unsupported action %q", foreign.field("name"), rule)
					}
				}
			}
			definitions = append(definitions, definition)
		}
		checks, err := archive.workbenchChecks(table)
		if err != nil {
			return nil, err
		}
		definitions = append(definitions, checks...)
		fmt.Fprintf(&sql, "CREATE TABLE %s (%s)", identifier(table.field("name")), strings.Join(definitions, ",\n"))
		engine := table.field("tableEngine")
		if engine == "" {
			engine = "InnoDB"
		}
		if !strings.EqualFold(engine, "InnoDB") {
			return nil, fmt.Errorf("Workbench table %q: only InnoDB is supported", table.field("name"))
		}
		sql.WriteString(" ENGINE=InnoDB")
		charset, collation := table.field("defaultCharacterSetName"), table.field("defaultCollationName")
		if charset == "" && collation == "" {
			charset, collation = schema.field("defaultCharacterSetName"), schema.field("defaultCollationName")
		}
		for _, option := range []struct{ value, sql string }{{charset, " DEFAULT CHARSET="}, {collation, " COLLATE="}} {
			if option.value != "" {
				if !mysqlWord.MatchString(option.value) {
					return nil, fmt.Errorf("Workbench table %q has invalid charset or collation", table.field("name"))
				}
				sql.WriteString(option.sql + option.value)
			}
		}
		if comment := table.field("comment"); comment != "" {
			sql.WriteString(" COMMENT=" + literal(comment))
		}
		sql.WriteString(";\n")
	}
	document, err := ImportSQL(sql.String(), name)
	if err != nil {
		return nil, fmt.Errorf("Workbench schema: %w", err)
	}
	document.Charset, document.Collation = schema.field("defaultCharacterSetName"), schema.field("defaultCollationName")
	for i := range document.Tables {
		for _, table := range tables {
			if document.Tables[i].Name == table.field("name") && table.field("defaultCharacterSetName") == "" && table.field("defaultCollationName") == "" {
				document.Tables[i].Charset, document.Tables[i].Collation = "", ""
			}
		}
	}
	if hasDiagram {
		if err := archive.applyLayout(document); err != nil {
			return nil, err
		}
	}
	if err := Validate(document); err != nil {
		return nil, err
	}
	return document, nil
}

func (node *workbenchNode) excluded() bool {
	return node.field("modelOnly") == "1" || node.field("commentedOut") == "1"
}

func (node *workbenchNode) unsupported(keys ...string) error {
	for _, key := range keys {
		property := node.property(key)
		if property != nil && (len(property.Children) > 0 || strings.TrimSpace(property.Text) != "" && property.Text != "0") {
			return fmt.Errorf("Workbench %s %q: unsupported %s", node.StructName, node.field("name"), key)
		}
	}
	return nil
}

func (archive *workbenchDocument) members(node *workbenchNode, key, class string) ([]*workbenchNode, error) {
	property := node.property(key)
	result := []*workbenchNode{}
	if property == nil {
		return result, nil
	}
	for _, child := range property.Children {
		id := child.ID
		if id == "" {
			id = strings.TrimSpace(child.Text)
		}
		member := archive.objects[id]
		if member == nil || member.StructName != class {
			return nil, fmt.Errorf("Workbench %s %q has a missing or invalid %s reference %q", node.StructName, node.field("name"), key, id)
		}
		result = append(result, member)
	}
	return result, nil
}

func (archive *workbenchDocument) columnNames(node *workbenchNode, key string, names map[string]string) ([]string, error) {
	members, err := archive.members(node, key, "db.mysql.Column")
	if err != nil {
		return nil, err
	}
	result := []string{}
	for _, member := range members {
		name, exists := names[member.ID]
		if !exists {
			return nil, fmt.Errorf("Workbench foreign key %q references a column outside its table", node.field("name"))
		}
		result = append(result, identifier(name))
	}
	return result, nil
}

func (archive *workbenchDocument) workbenchChecks(node *workbenchNode) ([]string, error) {
	checks, err := archive.members(node, "checks", "db.CheckConstraint")
	if err != nil {
		return nil, err
	}
	result := []string{}
	for _, check := range checks {
		definition := "CONSTRAINT " + identifier(check.field("name")) + " CHECK (" + check.field("searchCondition") + ")"
		if check.field("enforced") == "0" {
			definition += " NOT ENFORCED"
		}
		result = append(result, definition)
	}
	return result, nil
}

func (column *workbenchNode) workbenchColumnSQL() (string, error) {
	if err := column.unsupported("userType", "structuredType"); err != nil {
		return "", err
	}
	const typePrefix = "com.mysql.rdbms.mysql.datatype."
	reference := column.field("simpleType")
	if !strings.HasPrefix(reference, typePrefix) {
		return "", fmt.Errorf("Workbench column %q has unsupported type %q", column.field("name"), reference)
	}
	typeName := strings.TrimSuffix(strings.TrimPrefix(reference, typePrefix), "_f")
	switch typeName {
	case "tinyint", "smallint", "mediumint", "int", "integer", "bigint", "decimal", "numeric", "float", "double", "real", "bit", "bool", "boolean", "date", "datetime", "timestamp", "time", "year", "char", "varchar", "binary", "varbinary", "tinytext", "text", "mediumtext", "longtext", "tinyblob", "blob", "mediumblob", "longblob", "enum", "set", "json", "geometry", "point", "linestring", "polygon", "multipoint", "multilinestring", "multipolygon", "geometrycollection":
	default:
		return "", fmt.Errorf("Workbench column %q has unsupported type %q", column.field("name"), reference)
	}
	parameters := map[string]int{}
	for _, key := range []string{"length", "precision", "scale"} {
		value := -1
		if text := column.field(key); text != "" {
			parsed, err := strconv.Atoi(text)
			if err != nil || parsed < -1 {
				return "", fmt.Errorf("Workbench column %q has invalid %s %q", column.field("name"), key, text)
			}
			value = parsed
		}
		parameters[key] = value
	}
	typeSQL := strings.ToUpper(typeName)
	if explicit := strings.TrimSpace(column.field("datatypeExplicitParams")); explicit != "" {
		typeSQL += explicit
	} else {
		length := parameters["length"]
		switch typeName {
		case "decimal", "numeric", "float", "double", "real":
			length = parameters["precision"]
		}
		if length < 0 && parameters["precision"] >= 0 {
			length = parameters["precision"]
		}
		if length >= 0 {
			typeSQL += "(" + strconv.Itoa(length)
			if parameters["scale"] >= 0 {
				typeSQL += "," + strconv.Itoa(parameters["scale"])
			}
			typeSQL += ")"
		}
	}
	if flags := column.property("flags"); flags != nil {
		for _, flag := range flags.Children {
			switch strings.ToUpper(strings.TrimSpace(flag.Text)) {
			case "UNSIGNED":
				typeSQL += " UNSIGNED"
			default:
				return "", fmt.Errorf("Workbench column %q has unsupported flag %q", column.field("name"), flag.Text)
			}
		}
	}
	definition := identifier(column.field("name")) + " " + typeSQL
	for _, option := range []struct{ key, sql string }{{"characterSetName", " CHARACTER SET "}, {"collationName", " COLLATE "}} {
		if value := column.field(option.key); value != "" {
			if !mysqlWord.MatchString(value) {
				return "", fmt.Errorf("Workbench column %q has invalid charset or collation", column.field("name"))
			}
			definition += option.sql + value
		}
	}
	if column.field("generated") == "1" {
		definition += " GENERATED ALWAYS AS (" + column.field("expression") + ")"
		storage := strings.ToUpper(column.field("generatedStorage"))
		if storage != "" && storage != "VIRTUAL" && storage != "STORED" {
			return "", fmt.Errorf("Workbench column %q has unsupported generated storage %q", column.field("name"), storage)
		}
		if storage != "" {
			definition += " " + storage
		}
	} else if err := column.unsupported("expression", "generatedStorage"); err != nil {
		return "", err
	}
	if column.field("isNotNull") == "1" {
		definition += " NOT NULL"
	} else {
		definition += " NULL"
	}
	value := strings.TrimSpace(column.field("defaultValue"))
	if column.field("defaultValueIsNull") == "1" {
		value = "NULL"
	}
	if value != "" {
		if strings.HasPrefix(strings.ToUpper(value), "ON UPDATE ") {
			definition += " " + value
		} else {
			definition += " DEFAULT " + value
		}
	}
	if column.field("autoIncrement") == "1" {
		definition += " AUTO_INCREMENT"
	}
	if column.field("invisible") == "1" {
		definition += " INVISIBLE"
	}
	if comment := column.field("comment"); comment != "" {
		definition += " COMMENT " + literal(comment)
	}
	// SQL-valued properties may describe expressions, but cannot introduce
	// objects absent from the Workbench catalog.
	statement, err := sqlparser.NewTestParser().ParseStrictDDL("CREATE TABLE `_workbench_column` (" + definition + ")")
	if err != nil {
		return "", fmt.Errorf("Workbench column %q: %w", column.field("name"), err)
	}
	create, ok := statement.(*sqlparser.CreateTable)
	if !ok || create.TableSpec == nil || len(create.TableSpec.Columns) != 1 || len(create.TableSpec.Indexes) != 0 || len(create.TableSpec.Constraints) != 0 || len(create.TableSpec.Options) != 0 || create.TableSpec.PartitionOption != nil {
		return "", fmt.Errorf("Workbench column %q must contain one column definition", column.field("name"))
	}
	if options := create.TableSpec.Columns[0].Type.Options; options != nil && options.KeyOpt != sqlparser.ColKeyNone {
		return "", fmt.Errorf("Workbench column %q cannot define an inline index", column.field("name"))
	}
	return definition, nil
}
