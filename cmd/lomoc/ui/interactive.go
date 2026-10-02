package ui

import (
	"log"
	"os"
	"sync"

	"bitbucket.org/lomoware/lomo-backend/common/scan"
	"github.com/gdamore/tcell"
)

// Interactive starts interactive UI to allow user to select
func Interactive(rootFolder *scan.File) error {
	s := initScreen()
	commands := make(chan Executer)
	states := make(chan State)
	lastStateChan := make(chan *State, 1)
	var wg sync.WaitGroup
	wg.Add(3)
	go startProcessing(rootFolder, commands, states, lastStateChan, &wg)
	go interactiveFolder(s, states, &wg)
	go parseCommand(s, commands, &wg)
	wg.Wait()
	s.Fini()
	//lastState := <-lastStateChan
	//printMarkedFiles(lastState, *nullTerminate)
	return nil
}

func initScreen() tcell.Screen {
	tcell.SetEncodingFallback(tcell.EncodingFallbackASCII)
	s, e := tcell.NewScreen()
	if e != nil {
		log.Printf("%v\n", e)
		os.Exit(1)
	}
	if e = s.Init(); e != nil {
		log.Printf("%v\n", e)
		os.Exit(1)
	}
	s.Clear()
	return s
}

// startProcessing reads user commands and applies them to state
func startProcessing(
	folder *scan.File,
	commands <-chan Executer,
	states chan<- State,
	lastStateChan chan<- *State,
	wg *sync.WaitGroup,
) {
	defer wg.Done()
	state := State{
		Folder:      folder,
		MarkedFiles: make(map[*scan.File]struct{}),
	}
	states <- state
	for {
		command, more := <-commands
		if !more {
			close(states)
			break
		}
		if newState, err := command.Execute(state); err == nil {
			state = newState
			states <- state
		}
	}
	lastStateChan <- &state
}
