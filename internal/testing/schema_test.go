// Package testing_test compares generated schemas with Vitess schemadiff and
// MySQL itself. The expected definitions come directly from SQL fixtures rather
// than Sciurus's schema normalization or SQL rendering.
package testing_test

import (
	"context"
	cryptorand "crypto/rand"
	"database/sql"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/mhemmings/sciurus/examples"
	"github.com/mhemmings/sciurus/internal/model"
	"github.com/stretchr/testify/require"
	"vitess.io/vitess/go/vt/schemadiff"
	"vitess.io/vitess/go/vt/sqlparser"
)

// schemaSnapshot records server-normalized definitions and inherited defaults.
// Table order is fixed by the query; CREATE text retains column and index order.
type schemaSnapshot struct {
	Charset   string
	Collation string
	Names     []string
	Creates   []string
}

// TestSchemaEquivalence verifies example schemas and deliberately varied
// features after SQL import, JSON save/load, and SQL generation. Schemadiff runs
// without a database; DB_ADDR enables additional live MySQL checks.
func TestSchemaEquivalence(t *testing.T) {
	if os.Getenv("REQUIRE_MYSQL") == "1" {
		require.NotEmpty(t, os.Getenv("DB_ADDR"), "REQUIRE_MYSQL=1 requires DB_ADDR and a running MySQL server")
		require.False(t, testing.Short(), "REQUIRE_MYSQL=1 cannot be combined with -short")
	}
	var config *mysql.Config
	if os.Getenv("DB_ADDR") != "" && !testing.Short() {
		config = mysql.NewConfig()
		config.Net = "tcp"
		config.Addr = os.Getenv("DB_ADDR")
		config.User = os.Getenv("DB_USER")
		if config.User == "" {
			config.User = "root"
		}
		config.Passwd = os.Getenv("DB_PASS")
		config.MultiStatements = true
		config.Timeout = 5 * time.Second
		config.ReadTimeout = 30 * time.Second
		config.WriteTimeout = 30 * time.Second
	}
	// schemaCase pairs independent source DDL with a stable subtest name.
	type schemaCase struct {
		name string
		sql  string
		// Schemadiff requires unquoted charset names and table-level KEY/UNIQUE
		// declarations. These fixtures state that equivalent form independently.
		comparisonSQL string
	}
	cases := []schemaCase{
		{
			name: "literal_types",
			sql: "CREATE TABLE literals (" +
				"id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, " +
				"flag TINYINT(1) NOT NULL DEFAULT 0, " +
				"amount DECIMAL(20,6) NOT NULL DEFAULT -12.125, " +
				"status ENUM('plain','it''s','comma,value') NOT NULL DEFAULT 'plain', " +
				"tags SET('a','b') NULL DEFAULT NULL, " +
				"empty_string VARCHAR(64) NOT NULL DEFAULT '', " +
				"empty_text TEXT NOT NULL DEFAULT (''), " +
				"quoted VARCHAR(64) NOT NULL DEFAULT 'O''Reilly', " +
				"bits BIT(8) NOT NULL DEFAULT b'00000001', " +
				"binary_data VARBINARY(32) NULL, payload JSON NULL, " +
				"notes MEDIUMTEXT NULL COMMENT 'human readable', PRIMARY KEY (id)) " +
				"ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin COMMENT='Literal coverage';",
		},
		{
			name: "index_order_prefix_visibility",
			sql: "CREATE TABLE indexed (id BIGINT NOT NULL, tenant INT NOT NULL, " +
				"payload VARCHAR(255) NULL, rank_value INT NULL, PRIMARY KEY (id), " +
				"UNIQUE KEY tenant_payload (tenant,payload), " +
				"KEY search_payload (payload(48) DESC) INVISIBLE COMMENT 'prefix search', " +
				"KEY rank_order (rank_value DESC,tenant ASC) VISIBLE) ENGINE=InnoDB;",
		},
		{
			name: "composite_foreign_keys",
			sql: "CREATE TABLE parent (tenant INT UNSIGNED NOT NULL, code CHAR(2) " +
				"CHARACTER SET 'ascii' COLLATE 'ascii_bin' NOT NULL, PRIMARY KEY (tenant,code)) ENGINE=InnoDB;" +
				"CREATE TABLE child (id BIGINT NOT NULL, tenant INT UNSIGNED NULL, code CHAR(2) " +
				"CHARACTER SET 'ascii' COLLATE 'ascii_bin' NULL, PRIMARY KEY (id), " +
				"KEY child_parent (tenant,code), CONSTRAINT child_parent_fk FOREIGN KEY (tenant,code) " +
				"REFERENCES parent (tenant,code) ON DELETE SET NULL ON UPDATE CASCADE) ENGINE=InnoDB;",
			comparisonSQL: "CREATE TABLE parent (tenant INT UNSIGNED NOT NULL, " +
				"code CHAR(2) CHARACTER SET ascii COLLATE ascii_bin NOT NULL, PRIMARY KEY (tenant,code)) ENGINE=InnoDB;" +
				"CREATE TABLE child (id BIGINT NOT NULL, tenant INT UNSIGNED NULL, " +
				"code CHAR(2) CHARACTER SET ascii COLLATE ascii_bin NULL, PRIMARY KEY (id), " +
				"KEY child_parent (tenant,code), CONSTRAINT child_parent_fk FOREIGN KEY (tenant,code) " +
				"REFERENCES parent (tenant,code) ON DELETE SET NULL ON UPDATE CASCADE) ENGINE=InnoDB;",
		},
		{
			name: "cyclic_foreign_keys",
			sql: "SET @OLD_FOREIGN_KEY_CHECKS=@@FOREIGN_KEY_CHECKS, FOREIGN_KEY_CHECKS=0;" +
				"CREATE TABLE first_table (id INT NOT NULL, second_id INT NULL, PRIMARY KEY (id), " +
				"KEY first_second (second_id), CONSTRAINT first_second_fk FOREIGN KEY (second_id) " +
				"REFERENCES second_table (id) ON DELETE SET NULL) ENGINE=InnoDB;" +
				"CREATE TABLE second_table (id INT NOT NULL, first_id INT NULL, PRIMARY KEY (id), " +
				"KEY second_first (first_id), CONSTRAINT second_first_fk FOREIGN KEY (first_id) " +
				"REFERENCES first_table (id) ON DELETE SET NULL) ENGINE=InnoDB;SET FOREIGN_KEY_CHECKS=@OLD_FOREIGN_KEY_CHECKS;",
		},
		{
			name: "generated_checks_temporal",
			sql: "CREATE TABLE calculated (id BIGINT NOT NULL, price DECIMAL(12,2) NOT NULL, " +
				"quantity INT NOT NULL DEFAULT 1, total DECIMAL(18,2) GENERATED ALWAYS AS (price * quantity) STORED, " +
				"doubled INT GENERATED ALWAYS AS (quantity * 2) VIRTUAL, " +
				"updated DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6), " +
				"internal_value INT INVISIBLE DEFAULT 0, PRIMARY KEY (id), " +
				"CONSTRAINT positive_price CHECK (price >= 0), " +
				"CONSTRAINT positive_quantity CHECK (quantity > 0) NOT ENFORCED) ENGINE=InnoDB;",
		},
		{
			name: "text_spatial_indexes",
			sql: "CREATE TABLE searchable (id INT NOT NULL, body TEXT NULL, location POINT NOT NULL, " +
				"PRIMARY KEY (id), FULLTEXT KEY body_search (body), SPATIAL KEY location_search (location)) ENGINE=InnoDB;",
		},
		{
			name: "quoted_identifiers",
			sql: "CREATE TABLE `order` (`select` BIGINT NOT NULL, `tick``name` VARCHAR(64) NULL, " +
				"PRIMARY KEY (`select`), KEY `odd``index` (`tick``name`)) ENGINE=InnoDB;",
		},
		{
			name: "inline_keys_expression_defaults",
			sql: "CREATE TABLE inline_keys (id BIGINT NOT NULL KEY, email VARCHAR(80) NOT NULL UNIQUE KEY, " +
				"code CHAR(20) NOT NULL UNIQUE, quantity INT NOT NULL DEFAULT (1 + 2), " +
				"metadata JSON NOT NULL DEFAULT (JSON_OBJECT('enabled',true,'label','it''s')), " +
				"normalized_email VARCHAR(80) GENERATED ALWAYS AS (LOWER(email)) STORED) ENGINE=InnoDB;",
			comparisonSQL: "CREATE TABLE inline_keys (id BIGINT NOT NULL, email VARCHAR(80) NOT NULL, " +
				"code CHAR(20) NOT NULL, quantity INT NOT NULL DEFAULT (1 + 2), " +
				"metadata JSON NOT NULL DEFAULT (JSON_OBJECT('enabled',true,'label','it''s')), " +
				"normalized_email VARCHAR(80) GENERATED ALWAYS AS (LOWER(email)) STORED, " +
				"PRIMARY KEY (id), UNIQUE KEY email (email), UNIQUE KEY code (code)) ENGINE=InnoDB;",
		},
		{
			name: "document_charset_inheritance",
			sql: "CREATE TABLE inherited (id INT NOT NULL, name VARCHAR(40) NOT NULL, PRIMARY KEY (id)) " +
				"ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;" +
				"CREATE TABLE collation_only (id INT NOT NULL, name VARCHAR(40) NOT NULL, PRIMARY KEY (id)) " +
				"ENGINE=InnoDB COLLATE=latin1_swedish_ci;" +
				"CREATE TABLE charset_only (id INT NOT NULL, name VARCHAR(40) NOT NULL, PRIMARY KEY (id)) " +
				"ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;",
		},
	}
	// The larger example has independently authored DDL, so it does not use the
	// generator as its own oracle. Its bundled model is compared directly with
	// that DDL below. The starter exercises SQL import and a JSON round trip.
	projectSQL, err := os.ReadFile(filepath.Join("..", "..", "examples", "project-management.sql"))
	require.NoError(t, err)
	cases = append(cases, schemaCase{name: "project_management", sql: string(projectSQL)})
	starterJSON, err := examples.Models.ReadFile("starter.json")
	require.NoError(t, err)
	starter, err := model.Decode(starterJSON)
	require.NoError(t, err)
	starterSQL, err := model.Generate(starter, time.Time{})
	require.NoError(t, err)
	cases = append(cases, schemaCase{name: "commerce", sql: string(starterSQL)})
	// Build twelve independent SQL corpora without using any model or generator
	// helper. Each seed changes types, defaults, nullability, collations, index
	// details, table order and referential actions across three to five tables.
	// A ring of composite FKs plus self references exercises cyclic creation and
	// reference matching beyond a single hand-selected example.
	for seed := range 12 {
		random := rand.New(rand.NewSource(int64(seed)))
		count := 3 + seed%3
		var source strings.Builder
		source.WriteString("SET @OLD_FOREIGN_KEY_CHECKS=@@FOREIGN_KEY_CHECKS, FOREIGN_KEY_CHECKS=0;\n")
		for _, table := range random.Perm(count) {
			name := fmt.Sprintf("seed_%02d_table_%d", seed, table)
			parent := fmt.Sprintf("seed_%02d_table_%d", seed, (table+1)%count)
			action := []string{"CASCADE", "SET NULL", "RESTRICT"}[random.Intn(3)]
			collation := []string{"utf8mb4_bin", "utf8mb4_0900_ai_ci"}[random.Intn(2)]
			direction := []string{"ASC", "DESC"}[random.Intn(2)]
			visibility := []string{"VISIBLE", "INVISIBLE"}[random.Intn(2)]
			value := []string{
				fmt.Sprintf("INT UNSIGNED NOT NULL DEFAULT %d", random.Intn(100)),
				fmt.Sprintf("DECIMAL(%d,%d) NOT NULL DEFAULT -12.25", 12+seed%4, 2+seed%3),
				"ENUM('alpha','comma,value','quote''value') NOT NULL DEFAULT 'alpha'",
				"JSON NULL",
				"SET('red','blue','green') NOT NULL DEFAULT 'red,blue'",
				"VARBINARY(48) NULL",
				"BIT(8) NOT NULL DEFAULT b'101'",
				"DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)",
			}[(seed+table)%8]
			payload := fmt.Sprintf("VARCHAR(%d) NULL DEFAULT NULL", 80+random.Intn(80))
			if seed%2 == 0 {
				payload = "VARCHAR(120) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT ''"
			}
			fmt.Fprintf(&source, "CREATE TABLE `%s` (\n", name)
			source.WriteString("tenant_id INT UNSIGNED NOT NULL, id BIGINT UNSIGNED NOT NULL, parent_tenant_id INT UNSIGNED NULL, parent_id BIGINT UNSIGNED NULL, self_id BIGINT UNSIGNED NULL,\n")
			fmt.Fprintf(&source, "payload %s, varied_value %s, public_code CHAR(20) NOT NULL,\n", payload, value)
			source.WriteString("PRIMARY KEY (tenant_id,id), UNIQUE KEY public_code_unique (tenant_id,public_code), KEY parent_index (parent_tenant_id,parent_id), KEY self_index (tenant_id,self_id),\n")
			fmt.Fprintf(&source, "KEY payload_index (payload(%d) %s) %s,\n", 12+random.Intn(20), direction, visibility)
			fmt.Fprintf(&source, "CONSTRAINT `%s_parent` FOREIGN KEY (parent_tenant_id,parent_id) REFERENCES `%s` (tenant_id,id) ON DELETE %s ON UPDATE %s,\n", name, parent, action, action)
			fmt.Fprintf(&source, "CONSTRAINT `%s_self` FOREIGN KEY (tenant_id,self_id) REFERENCES `%s` (tenant_id,id) ON DELETE CASCADE ON UPDATE NO ACTION\n", name, name)
			fmt.Fprintf(&source, ") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=%s COMMENT='Corpus seed %d, table %d';\n", collation, seed, table)
		}
		source.WriteString("SET FOREIGN_KEY_CHECKS=@OLD_FOREIGN_KEY_CHECKS;\n")
		cases = append(cases, schemaCase{name: fmt.Sprintf("generated_seed_%02d", seed), sql: source.String()})
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			// A JSON round trip exercises the actual saved-document contract, rather
			// than accidentally testing a richer in-memory import representation.
			var doc *model.Document
			if testCase.name == "project_management" {
				// Verify that the hand-authored SQL and example model agree, rather
				// than comparing only a model imported from that same SQL source.
				contents, readErr := examples.Models.ReadFile("project-management.er.json")
				require.NoError(t, readErr)
				var decodeErr error
				doc, decodeErr = model.Decode(contents)
				require.NoError(t, decodeErr)
			} else {
				var importErr error
				doc, importErr = model.ImportSQL(testCase.sql, testCase.name)
				require.NoError(t, importErr)
			}
			if testCase.name == "document_charset_inheritance" {
				// The baseline states the inherited table's defaults explicitly.
				// Express those defaults at document level instead, then check that
				// neither a table-only charset nor a table-only collation inherits
				// the other half of that pair. MySQL resolves each override itself.
				doc.Charset = "utf8mb4"
				doc.Collation = "utf8mb4_bin"
				for i := range doc.Tables {
					if doc.Tables[i].Name == "inherited" {
						doc.Tables[i].Charset = ""
						doc.Tables[i].Collation = ""
					}
				}
			}
			encoded, encodeErr := model.Encode(doc)
			require.NoError(t, encodeErr)
			doc, decodeErr := model.Decode(encoded)
			require.NoError(t, decodeErr)
			generated, generateErr := model.Generate(doc, time.Unix(0, 0).UTC())
			require.NoError(t, generateErr)

			comparisonSQL := testCase.comparisonSQL
			if comparisonSQL == "" {
				comparisonSQL = testCase.sql
			}
			compareWithSchemadiff(t, comparisonSQL, string(generated))
			t.Run("mysql", func(t *testing.T) {
				if config == nil {
					t.Skip("set DB_ADDR without -short, or run ./test.sh, for live MySQL tests")
				}
				sourceDB := createDatabase(t, config, testCase.sql)
				targetDB := createDatabase(t, config, string(generated))
				source := readSchema(t, sourceDB)
				target := readSchema(t, targetDB)
				require.Equal(t, source, target, "MySQL normalized schema differs after model round trip")
				t.Logf("MySQL SHOW CREATE: %d tables identical", len(source.Names))
			})
		})
	}
}

// createDatabase initializes a uniquely named test database and drops only that
// database during cleanup. Register ownership immediately after each successful
// acquisition so failed connections or schema initialization also clean up.
func createDatabase(t *testing.T, config *mysql.Config, schema string) *sql.DB {
	t.Helper()
	admin, err := sql.Open("mysql", config.FormatDSN())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, admin.Close()) })
	var identifier [12]byte
	_, err = cryptorand.Read(identifier[:])
	require.NoError(t, err)
	name := fmt.Sprintf("sciurus_test_%x", identifier)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err = admin.ExecContext(ctx, "CREATE DATABASE `"+name+"`")
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		_, dropErr := admin.ExecContext(cleanupCtx, "DROP DATABASE `"+name+"`")
		require.NoError(t, dropErr, "remove disposable test database %s", name)
	})
	connection := config.Clone()
	connection.DBName = name
	db, err := sql.Open("mysql", connection.FormatDSN())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	_, err = db.ExecContext(ctx, schema)
	require.NoError(t, err)
	return db
}

// readSchema takes a deterministic snapshot of MySQL's own table definitions.
func readSchema(t *testing.T, db *sql.DB) schemaSnapshot {
	t.Helper()
	var snapshot schemaSnapshot
	err := db.QueryRow("SELECT DEFAULT_CHARACTER_SET_NAME, DEFAULT_COLLATION_NAME FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = DATABASE()").Scan(&snapshot.Charset, &snapshot.Collation)
	require.NoError(t, err)
	rows, err := db.Query("SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() ORDER BY BINARY TABLE_NAME")
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var name string
		err = rows.Scan(&name)
		require.NoError(t, err)
		snapshot.Names = append(snapshot.Names, name)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	// Close the listing cursor before opening more queries; this also makes
	// resource ownership explicit before the database is eventually dropped.
	for _, name := range snapshot.Names {
		var returnedName, statement string
		err = db.QueryRow("SHOW CREATE TABLE `"+strings.ReplaceAll(name, "`", "``")+"`").Scan(&returnedName, &statement)
		require.NoError(t, err)
		require.Equal(t, name, returnedName)
		snapshot.Creates = append(snapshot.Creates, statement)
	}
	return snapshot
}

// compareWithSchemadiff reads the original definitions directly. SET statements
// control MySQL execution and do not describe the schema being compared.
func compareWithSchemadiff(t *testing.T, sourceSQL, generatedSQL string) {
	t.Helper()
	env := schemadiff.NewTestEnv()
	var schemas [2]*schemadiff.Schema
	for i, source := range []string{sourceSQL, generatedSQL} {
		statements, err := env.Parser().ParseMultipleIgnoreEmpty(source)
		require.NoError(t, err)
		definitions := statements[:0]
		for _, statement := range statements {
			if _, isSet := statement.(*sqlparser.Set); !isSet {
				definitions = append(definitions, statement)
			}
		}
		schemas[i], err = schemadiff.NewSchemaFromStatements(env, definitions)
		require.NoError(t, err)
	}
	hints := &schemadiff.DiffHints{
		ConstraintNamesStrategy: schemadiff.ConstraintNamesStrict,
		AutoIncrementStrategy:   schemadiff.AutoIncrementApplyAlways,
	}
	for direction := range 2 {
		diff, err := schemadiff.DiffSchemas(env, schemas[direction], schemas[1-direction], hints)
		require.NoError(t, err)
		var changes []string
		for _, change := range diff.UnorderedDiffs() {
			changes = append(changes, change.CanonicalStatementString())
		}
		require.Empty(t, changes, "schemadiff found schema changes (direction %d)", direction)
	}
}
