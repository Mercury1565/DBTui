package main

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func (s *AppState) loadSchemas() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, err := s.pool.Query(ctx, `
		SELECT schema_name
		FROM information_schema.schemata
		WHERE schema_name NOT IN ('pg_toast', 'pg_temp_1', 'pg_toast_temp_1')
		ORDER BY schema_name`)
	if err != nil {
		return err
	}
	defer rows.Close()

	s.schemaList.Clear()
	schemas := []string{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return err
		}
		schemas = append(schemas, n)
	}
	sort.Strings(schemas)
	for i, sc := range schemas {
		s.schemaList.AddItem(sc, "", rune('a'+(i%26)), nil)
	}
	if len(schemas) > 0 {
		s.currentSchema = schemas[0]
		s.loadTables(s.currentSchema)
	}
	return rows.Err()
}

func (s *AppState) loadTables(schema string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, err := s.pool.Query(ctx, `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = $1 AND table_type = 'BASE TABLE'
		ORDER BY table_name`, schema)
	if err != nil {
		s.toast("load tables: %v", err)
		return
	}
	defer rows.Close()

	s.tableList.Clear()
	tables := []string{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			s.toast("scan: %v", err)
			return
		}
		tables = append(tables, n)
	}
	for i, t := range tables {
		s.tableList.AddItem(t, "", rune('a'+(i%26)), nil)
	}
	if len(tables) > 0 {
		s.currentTable = tables[0]
		s.loadColumns(schema, tables[0])
		s.previewTable(schema, tables[0])
	} else {
		s.columnTable.Clear()
		s.resultTable.Clear()
	}
}

func (s *AppState) loadColumns(schema, table string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, err := s.pool.Query(ctx, `
		SELECT column_name, data_type, is_nullable
		FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = $2
		ORDER BY ordinal_position`, schema, table)
	if err != nil {
		s.toast("load columns: %v", err)
		return
	}
	defer rows.Close()

	s.columnTable.Clear()
	setHeader(s.columnTable, []string{"Column", "Type", "Nullable"})
	row := 1
	for rows.Next() {
		var name, typ, nullable string
		if err := rows.Scan(&name, &typ, &nullable); err != nil {
			s.toast("scan: %v", err)
			return
		}
		setRow(s.columnTable, row, []string{name, typ, nullable})
		row++
	}
}

func (s *AppState) previewTable(schema, table string) {
	ident := pgIdent(schema) + "." + pgIdent(table)
	q := fmt.Sprintf("SELECT * FROM %s LIMIT %d", ident, s.previewLimit)
	s.runQueryInto(q, s.resultTable)
}

func (s *AppState) runAdhocQuery(q string) {
	q = strings.TrimSpace(q)
	if q == "" {
		return
	}
	// Basic defense: prevent multiple statements to avoid surprises in a TUI.
	if hasMultipleStatements(q) {
		s.toast("Multiple statements detected; please run one at a time.")
		return
	}
	s.runQueryInto(q, s.resultTable)
}

func (s *AppState) runQueryInto(q string, tbl *tview.Table) {
	started := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		s.toast("query error: %v", err)
		return
	}
	defer rows.Close()

	flds := rows.FieldDescriptions()
	headers := make([]string, len(flds))
	for i, f := range flds {
		headers[i] = string(f.Name)
	}
	tbl.Clear()
	setHeader(tbl, headers)
	row := 1
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			s.toast("row error: %v", err)
			return
		}
		cells := make([]string, len(vals))
		for i, v := range vals {
			if v == nil {
				cells[i] = "NULL"
				continue
			}
			cells[i] = fmt.Sprint(v)
		}
		setRow(tbl, row, cells)
		row++
	}
	if err := rows.Err(); err != nil {
		s.toast("rows err: %v", err)
		return
	}
	s.resultTable.ScrollToBeginning()
	s.toast("%d rows in %s", row-1, time.Since(started).Truncate(time.Millisecond))
}

func setHeader(t *tview.Table, cols []string) {
	for i, h := range cols {
		text := fmt.Sprintf(" %s ", h)
		if i > 0 {
			text = fmt.Sprintf(" | %s ", h)
		}
		cell := tview.NewTableCell(text).SetSelectable(false).
			SetAttributes(tcell.AttrBold).
			SetTextColor(tcell.ColorWhite).
			SetBackgroundColor(tcell.ColorBlue)
		t.SetCell(0, i, cell)
	}
}

func setRow(t *tview.Table, row int, cols []string) {
	for i, v := range cols {
		text := fmt.Sprintf("%s", v)
		if i > 0 {
			text = fmt.Sprintf(" | %s", v)
		}
		t.SetCell(row, i, tview.NewTableCell(text).SetExpansion(1))
	}
}

var multiStmtRe = regexp.MustCompile(`;\s*[^\s]`)

func hasMultipleStatements(q string) bool { return multiStmtRe.FindStringIndex(q) != nil }

func pgIdent(name string) string {
	// Quote identifiers that need it
	if name == "" {
		return name
	}
	if regexp.MustCompile(`^[a-z_][a-z0-9_]*$`).MatchString(name) {
		return name
	}
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
