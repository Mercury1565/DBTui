// pg_tui.go
// A lightweight PostgreSQL TUI visualizer built with Go, tview, and pgx.
// Features:
// - Browse schemas → tables → columns
// - Preview table data (first N rows)
// - Run ad-hoc SQL queries in an input box; results render in a grid
// - Keyboard shortcuts: q (quit), F5 (run query), r (refresh list), Tab (cycle focus)
//
// Build:
//
//	go mod init pg_tui && \
//	go get github.com/rivo/tview github.com/gdamore/tcell/v2 github.com/jackc/pgx/v5/pgxpool && \
//	go build -o pg-tui
//
// Run (uses DATABASE_URL if no flags provided):
//
//	DATABASE_URL=postgres://user:pass@host:5432/dbname?sslmode=disable ./pg-tui
//	./pg-tui -url "postgres://user:pass@host:5432/dbname?sslmode=disable" -limit 200
//
// Notes:
// - Safe preview: SELECT * FROM schema.table LIMIT N (configurable)
// - Simple error toasts at bottom when queries fail
// - Read-only by default unless you type writes in the query box
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rivo/tview"
)

type AppState struct {
	app           *tview.Application
	pool          *pgxpool.Pool
	schemaList    *tview.List
	tableList     *tview.List
	columnTable   *tview.Table
	resultTable   *tview.Table
	queryArea     *tview.TextArea
	statusBar     *tview.TextView
	layout        *tview.Flex
	previewLimit  int
	currentSchema string
	currentTable  string
}

func main() {
	urlFlag := flag.String("url", "", "PostgreSQL connection URL (overrides $DATABASE_URL)")
	limit := flag.Int("limit", 100, "Row limit for previews")
	flag.Parse()

	connURL := *urlFlag
	if connURL == "" {
		connURL = os.Getenv("DATABASE_URL")
	}
	if connURL == "" {
		fmt.Println("Provide -url or set DATABASE_URL.")
		os.Exit(1)
	}

	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(connURL)
	if err != nil {
		log.Fatalf("parse config: %v", err)
	}
	cfg.MaxConns = 5
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	app := tview.NewApplication()
	state := &AppState{
		app:          app,
		pool:         pool,
		previewLimit: *limit,
	}

	state.initUI()
	if err := state.loadSchemas(); err != nil {
		state.toast("Failed to load schemas: %v", err)
	}
	state.updateStatus("F5: Run | q: Quit | r: Refresh | Tab: Cycle Focus")

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

func (s *AppState) initUI() {
	s.schemaList = tview.NewList().ShowSecondaryText(false)
	s.schemaList.SetBorder(true).SetTitle(" Schemas ")
	s.schemaList.SetSelectedFunc(func(index int, mainText, secondary string, shortcut rune) {
		s.currentSchema = mainText
		s.loadTables(mainText)
	})

	s.tableList = tview.NewList().ShowSecondaryText(false)
	s.tableList.SetBorder(true).SetTitle(" Tables ")
	s.tableList.SetSelectedFunc(func(index int, mainText, secondary string, shortcut rune) {
		s.currentTable = mainText
		s.loadColumns(s.currentSchema, s.currentTable)
		s.previewTable(s.currentSchema, s.currentTable)
	})

	s.columnTable = tview.NewTable().SetBorders(false)
	s.columnTable.SetBorder(true).SetTitle(" Columns ")

	s.resultTable = tview.NewTable().SetFixed(1, 0)
	s.resultTable.SetBorder(true).SetTitle(" Results / Preview ")

	// Correctly initialize TextArea
	s.queryArea = tview.NewTextArea()
	s.queryArea.SetPlaceholder("Enter SQL query here (F5 to run)")
	s.queryArea.SetBorder(true)
	s.queryArea.SetTitle(" SQL Query ")

	s.queryArea.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// Run query on F5
		if event.Key() == tcell.KeyF5 {
			s.runAdhocQuery(s.queryArea.GetText())
			return nil
		}
		return event
	})

	s.statusBar = tview.NewTextView().SetDynamicColors(true)
	s.statusBar.SetBorder(true).SetTitle(" Status ")

	left := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(s.schemaList, 0, 1, true).
		AddItem(s.tableList, 0, 1, false)

	right := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(s.columnTable, 0, 1, false).
		AddItem(s.resultTable, 0, 3, false).
		AddItem(s.queryArea, 5, 0, false). // Increased size for multiline
		AddItem(s.statusBar, 1, 0, false)

	s.layout = tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(left, 35, 0, true).
		AddItem(right, 0, 1, false)

	// Global keybindings
	s.app.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		// Only handle global keybindings if the query input is not focused
		if s.app.GetFocus() != s.queryArea {
			switch ev.Rune() {
			case 'q', 'Q':
				s.app.Stop()
				return nil
			case 'r', 'R':
				s.loadSchemas()
				s.toast("Refreshed schemas and tables.")
				return nil
			}
		}

		// Handle F5 and Tab regardless of focus
		switch ev.Key() {
		case tcell.KeyF5:
			s.runAdhocQuery(s.queryArea.GetText())
			return nil
		case tcell.KeyTab:
			s.cycleFocus()
			return nil
		}
		return ev
	})

	s.app.SetRoot(s.layout, true)
}

func (s *AppState) cycleFocus() {
	p := s.app.GetFocus()
	switch p {
	case s.schemaList:
		s.app.SetFocus(s.tableList)
	case s.tableList:
		s.app.SetFocus(s.columnTable)
	case s.columnTable:
		s.app.SetFocus(s.resultTable)
	case s.resultTable:
		s.app.SetFocus(s.queryArea)
	default:
		s.app.SetFocus(s.schemaList)
	}
}

func (s *AppState) toast(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	s.statusBar.SetText(fmt.Sprintf("[yellow]%s", msg))
	go func() {
		time.Sleep(3 * time.Second)
		s.app.QueueUpdateDraw(func() {
			s.updateStatus("F5: Run | q: Quit | r: Refresh | Tab: Cycle Focus")
		})
	}()
}

func (s *AppState) updateStatus(msg string) {
	s.statusBar.SetText(msg)
}

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
