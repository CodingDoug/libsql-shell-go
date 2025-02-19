package shellcmd

import (
	"fmt"
	"strings"

	"github.com/libsql/libsql-shell-go/internal/db"
	"github.com/spf13/cobra"
)

var dumpCmd = &cobra.Command{
	Use:   ".dump",
	Short: "Render database content as SQL",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		config, ok := cmd.Context().Value(dbCtx{}).(*DbCmdConfig)
		if !ok {
			return fmt.Errorf("missing db connection")
		}

		fmt.Fprintln(config.OutF, "PRAGMA foreign_keys=OFF;")

		getTableNamesStatementResult, err := getDbTableNames(config)
		if err != nil {
			return err
		}

		err = dumpTables(getTableNamesStatementResult, config)
		if err != nil {
			return err
		}

		return nil
	},
}

func dumpTables(getTableStatementResult db.StatementResult, config *DbCmdConfig) error {
	for tableNameRowResult := range getTableStatementResult.RowCh {
		if tableNameRowResult.Err != nil {
			return tableNameRowResult.Err
		}
		formattedRow, err := db.FormatData(tableNameRowResult.Row, db.TABLE)
		if err != nil {
			return err
		}

		formattedTableName := formattedRow[0]

		createTableStmt, otherStmts, err := getTableSchema(config, formattedTableName)
		if err != nil {
			return err
		}

		fmt.Fprintln(config.OutF, createTableStmt)

		tableRecordsStatementResult, err := getTableRecords(config, formattedTableName)
		if err != nil {
			return err
		}

		err = dumpTableRecords(tableRecordsStatementResult, config, formattedTableName)
		if err != nil {
			return err
		}

		for _, stmt := range otherStmts {
			fmt.Fprintln(config.OutF, stmt)
		}
	}

	return nil
}

func dumpTableRecords(tableRecordsStatementResult db.StatementResult, config *DbCmdConfig, tableName string) error {
	for tableRecordsRowResult := range tableRecordsStatementResult.RowCh {
		if tableRecordsRowResult.Err != nil {
			return tableRecordsRowResult.Err
		}

		var formattedTableName = tableName
		if db.NeedsEscaping(tableName) {
			formattedTableName = "'" + db.EscapeSingleQuotes(tableName) + "'"
		}
		insertStatement := "INSERT INTO " + formattedTableName + " VALUES ("

		tableRecordsFormattedRow, err := db.FormatData(tableRecordsRowResult.Row, db.SQLITE)
		if err != nil {
			return err
		}

		insertStatement += strings.Join(tableRecordsFormattedRow, ", ")
		insertStatement += ");"
		fmt.Fprintln(config.OutF, insertStatement)
	}

	return nil
}

func getDbTableNames(config *DbCmdConfig) (db.StatementResult, error) {
	listTablesResult, err := config.Db.ExecuteStatements("SELECT name FROM sqlite_master WHERE type='table' and name not like 'sqlite_%' and name != '_litestream_seq' and name != '_litestream_lock' and name != 'libsql_wasm_func_table'")
	if err != nil {
		return db.StatementResult{}, err
	}

	statementResult := <-listTablesResult.StatementResultCh
	if statementResult.Err != nil {
		return db.StatementResult{}, statementResult.Err
	}

	return statementResult, nil
}

func getTableSchema(config *DbCmdConfig, tableName string) (createTable string, otherStmts []string, err error) {
	formattedTableName := db.EscapeSingleQuotes(tableName)
	tableInfoResult, err := config.Db.ExecuteStatements(
		fmt.Sprintf("SELECT type, sql || ';' FROM sqlite_master WHERE TBL_NAME='%s'", formattedTableName),
	)
	if err != nil {
		return "", nil, err
	}

	statementResult := <-tableInfoResult.StatementResultCh
	if statementResult.Err != nil {
		return "", nil, statementResult.Err
	}

	for statementRowResult := range statementResult.RowCh {
		if statementRowResult.Err != nil {
			return "", nil, statementResult.Err
		}

		formatted, err := db.FormatData(statementRowResult.Row, db.TABLE)
		if err != nil {
			return "", nil, fmt.Errorf("failed to format data: %w", err)
		}
		if len(formatted) != 2 {
			return "", nil, fmt.Errorf("expected 2 columns, got %d", len(formatted))
		}

		kind := formatted[0]
		sql := formatted[1]
		if kind == "table" {
			createTable = sql
			continue
		}

		otherStmts = append(otherStmts, sql)
	}

	return
}

func getTableRecords(config *DbCmdConfig, tableName string) (db.StatementResult, error) {
	formattedTableName := db.EscapeSingleQuotes(tableName)
	tableRecordsResult, err := config.Db.ExecuteStatements(
		fmt.Sprintf("SELECT * FROM '%s'", formattedTableName),
	)
	if err != nil {
		return db.StatementResult{}, err
	}

	statementResult := <-tableRecordsResult.StatementResultCh
	if statementResult.Err != nil {
		return db.StatementResult{}, statementResult.Err
	}

	return statementResult, nil
}
