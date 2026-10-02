package ui

import (
	"fmt"
	"sync"

	"bitbucket.org/lomoware/lomo-backend/common/scan"
	"github.com/gdamore/tcell"
	"github.com/gdamore/tcell/views"
)

func interactiveFolder(s tcell.Screen, states chan State, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		state, more := <-states
		if !more {
			break
		}
		printOptions(state, s)
	}
}

func printOptions(state State, s tcell.Screen) {
	var (
		back, forth views.Widget
		selected    *scan.File
		total       int
	)

	s.Clear()
	outer := views.NewBoxLayout(views.Vertical)
	inner := views.NewBoxLayout(views.Horizontal)

	middle := views.NewCellView()

	_, windowsHeight := s.Size()
	vs := newVisualState(state, windowsHeight-1)
	middle.SetModel(vs)
	backState, err := GoBack{}.Execute(state)
	if err == nil {
		backCell := views.NewCellView()
		backCell.SetModel(newVisualState(backState, windowsHeight-1))
		back = backCell
		if len(state.Folder.Children) > 0 {
			if len(state.Folder.Children[state.Selected].Children) > 0 {
				total = len(state.Folder.Children[state.Selected].Children)
			} else {
				total = len(state.Folder.Children)
			}
		}
	} else {
		back = views.NewText()
		total = len(state.Folder.Children[state.Selected].Children)
	}
	left := ""
	right := ""
	if len(state.Folder.Children) > 0 {
		selected = state.Folder.Children[state.Selected]
		if len(selected.Children) > 0 && !selected.Children[0].IsFakeFile() {
			left = fmt.Sprintf("Total Files: %d | FROM %d-%d-%d TO %d-%d-%d", total,
				selected.EarliestTime.Year(), selected.EarliestTime.Month(), selected.EarliestTime.Day(),
				selected.LatestTime.Year(), selected.LatestTime.Month(), selected.LatestTime.Day())
		}
		right = fmt.Sprintf("Create Time: %d-%d-%d  ", selected.CreateTime.Year(), selected.CreateTime.Month(), selected.CreateTime.Day())
	}
	forth = views.NewText()
	if len(state.Folder.Children) > 0 && !state.Folder.Children[0].IsFakeFile() {
		forthState, err := Enter{}.Execute(state)
		if err == nil {
			forthCell := views.NewCellView()
			forthCell.SetModel(newVisualState(forthState, windowsHeight-1))
			forth = forthCell
		}
	}

	statusBar := views.NewSimpleStyledTextBar()
	statusBar.SetLeft(left)
	statusBar.SetRight(right)

	_, height1 := vs.GetBounds()
	if height1 == 0 {
		fmt.Printf("inner length: %d\n, window length: %d\n", height1, windowsHeight)
	}
	outer.SetView(s)
	outer.AddWidget(inner, 1.0)
	outer.AddWidget(statusBar, 0.0)
	inner.AddWidget(back, 0.33)
	inner.AddWidget(middle, 0.33)
	inner.AddWidget(forth, 0.33)
	outer.Draw()
	s.Show()
}
