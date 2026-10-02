package ui

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"

	"bitbucket.org/lomoware/lomo-backend/common/scan"
)

// State represents system configuration after processing user input
type State struct {
	Folder      *scan.File
	Selected    int
	history     map[*scan.File]int // last cursor location in each folder
	MarkedFiles map[*scan.File]struct{}
}

// Executer represents a user action triggered on a State.
type Executer interface {
	Execute(State) (State, error)
}

// Enter is an action opening selected directory.
type Enter struct{}

// GoBack is an action returning to parent directory.
type GoBack struct{}

// Down is an action selecting next file in the list.
type Down struct{}

// Up is an action selecting previous file in the list.
type Up struct{}

// Mark is an action that saves current directory for later use.
type Mark struct{}

// Open is an action opening selected directory via system command.
type Open struct{}

// Number is number to press.
type Number struct {
	key rune
}

func copyState(state State) State {
	return State{
		Folder:      state.Folder,
		history:     state.history,
		Selected:    state.Selected,
		MarkedFiles: state.MarkedFiles,
	}
}

// Execute represents a user action triggered on a State.
func (d Down) Execute(oldState State) (State, error) {
	if oldState.Selected+2 > len(oldState.Folder.Children) {
		return oldState, errors.New("trying to go down below last file")
	}
	newState := copyState(oldState)
	newState.Selected = oldState.Selected + 1
	return newState, nil
}

// Execute represents a user action triggered on a State.
func (u Up) Execute(oldState State) (State, error) {
	if oldState.Folder.IsFakeFile() {
		return oldState, errors.New("this is fake file")
	}
	if oldState.Selected == 0 {
		return oldState, errors.New("trying to go above first file")
	}
	newState := copyState(oldState)
	newState.Selected = oldState.Selected - 1
	return newState, nil
}

// Execute represents a user action triggered on a State.
func (e Enter) Execute(oldState State) (State, error) {
	newFolder := oldState.Folder.Children[oldState.Selected]
	// last selected is 'Apply' Button
	if oldState.Folder.Children[0].IsFakeFile() && oldState.Selected == len(oldState.Folder.Children)-1 {
		newFolder = oldState.Folder
	}
	//if len(newFolder.Files) == 0 {
	//	return oldState, errors.New("Trying to enter empty file")
	//}
	newHistory := map[*scan.File]int{}
	for fp, selected := range oldState.history {
		newHistory[fp] = selected
	}
	newHistory[oldState.Folder] = oldState.Selected
	return State{
		Folder:      newFolder,
		history:     newHistory,
		Selected:    newHistory[newFolder],
		MarkedFiles: oldState.MarkedFiles,
	}, nil
}

// Execute represents a user action triggered on a State.
func (GoBack) Execute(oldState State) (State, error) {
	parentFolder := oldState.Folder.Parent
	if parentFolder == nil {
		return oldState, errors.New("trying to go back on root")
	}
	newHistory := map[*scan.File]int{}
	for fp, selected := range oldState.history {
		newHistory[fp] = selected
	}
	newHistory[oldState.Folder] = oldState.Selected
	return State{
		Folder:      parentFolder,
		history:     newHistory,
		Selected:    newHistory[parentFolder],
		MarkedFiles: oldState.MarkedFiles,
	}, nil
}

// Execute represents a user action triggered on a State.
func (m Mark) Execute(oldState State) (State, error) {
	newState := copyState(oldState)
	selectedFile := newState.Folder.Children[newState.Selected]
	if _, exists := newState.MarkedFiles[selectedFile]; exists {
		delete(newState.MarkedFiles, selectedFile)
	} else {
		newState.MarkedFiles[selectedFile] = struct{}{}
	}
	return newState, nil
}

// Execute represents a user action triggered on a State.
func (o Open) Execute(oldState State) (State, error) {
	n := filepath.Join(oldState.Folder.Path(), oldState.Folder.Children[oldState.Selected].Name)
	out, err := exec.Command("open", n).CombinedOutput()
	if err != nil {
		fmt.Printf("open %s got %s\n", n, string(out))
	}
	return oldState, err
}

// Execute represents a user action triggered on a State.
func (n Number) Execute(oldState State) (State, error) {
	newState := copyState(oldState)
	newState.Folder.Name += string(n.key)
	return newState, nil
}
