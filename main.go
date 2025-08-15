package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

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
