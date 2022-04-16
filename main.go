package main

import (
	"fmt"
	"log"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func Watcher(output chan fsnotify.Event) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	done := make(chan bool)
	go func() {
		defer watcher.Close()
		go func() {
			for {
				select {
				case event, ok := <-watcher.Events:
					if !ok {
						return
					}

					output <- event

				case err, ok := <-watcher.Errors:
					if !ok {
						return
					}
					log.Println("error:", err)
				}
			}
		}()

		err = watcher.Add("/dev/pts")
		if err != nil {
			log.Fatal(err)
		}
		<-done
	}()
	return nil
}

type pty struct {
	ops     int
	path    string
	removed bool
}

type TableData struct {
	sync.RWMutex
	ptys []pty
	loc  map[string]int
	tview.TableContentReadOnly
}

func (d *TableData) GetCell(row, column int) *tview.TableCell {
	d.RLock()
	defer d.RUnlock()

	if row < 0 || column < 0 {
		return nil
	}

	contents := ""

	color := tcell.ColorWhite
	if row < 1 {
		color = tcell.ColorYellow
		headingCols := []string{"pts", "ops count"}
		contents = headingCols[column]
	} else {
		if d.ptys[row-1].removed {
			color = tcell.ColorGray
		}

		switch column {
		case 0:
			contents = d.ptys[row-1].path
		case 1:
			contents = fmt.Sprintf("%d", d.ptys[row-1].ops)
		}
	}

	return tview.NewTableCell(contents).
		SetTextColor(color).
		SetAlign(tview.AlignCenter)
}

func (d *TableData) GetRowCount() int {
	return len(d.ptys) + 1
}

func (d *TableData) GetColumnCount() int {
	return 2
}

func main() {

	data := &TableData{loc: make(map[string]int)}
	app := tview.NewApplication()

	table := tview.NewTable().
		SetBorders(true).SetContent(data)

	table.Select(0, 0).SetFixed(1, 1).SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEscape {
			app.Stop()
		}
		if key == tcell.KeyEnter {
			table.SetSelectable(true, true)
		}
	}).SetSelectedFunc(func(row int, column int) {
		if row != 0 {
			table.GetCell(row, column).SetTextColor(tcell.ColorRed)
			table.SetSelectable(false, false)
		}
	})

	events := make(chan fsnotify.Event)
	err := Watcher(events)
	if err != nil {
		panic(err)
	}

	go func() {
		for {
			e := <-events
			data.Lock()

			v, ok := data.loc[e.Name]
			if !ok {
				data.ptys = append(data.ptys, pty{path: e.Name})
				v = len(data.ptys) - 1
				data.loc[e.Name] = v
			}
			currentPty := data.ptys[v]

			switch e.Op {
			case fsnotify.Create:
				currentPty.removed = false
			case fsnotify.Remove:
				currentPty.removed = true
			default:
				currentPty.ops++
			}

			data.ptys[v] = currentPty

			data.Unlock()

			app.QueueUpdateDraw(func() {})

		}
	}()

	if err := app.SetRoot(table, true).EnableMouse(true).Run(); err != nil {
		panic(err)
	}

}
