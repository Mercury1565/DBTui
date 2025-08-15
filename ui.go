package main

import (
	"fmt"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

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
