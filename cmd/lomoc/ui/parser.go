package ui

import (
	"sync"

	"github.com/gdamore/tcell"
)

func parseCommand(s tcell.Screen, commands chan Executer, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		ev := s.PollEvent()
		switch ev := ev.(type) {
		case *tcell.EventKey:
			switch ev.Key() {
			case tcell.KeyEscape, tcell.KeyCtrlC:
				close(commands)
				return
			case tcell.KeyEnter, tcell.KeyRight:
				commands <- Enter{}
			case tcell.KeyDown:
				commands <- Down{}
			case tcell.KeyUp:
				commands <- Up{}
			case tcell.KeyBackspace, tcell.KeyLeft:
				commands <- GoBack{}
			case tcell.KeyCtrlL:
				s.Sync()
			case tcell.KeyRune:
				key := ev.Rune()
				switch key {
				case ' ':
					commands <- Mark{}
				case 'q':
					close(commands)
					return
				case 'o':
					commands <- Open{}
				case 'h':
					commands <- GoBack{}
				case 'j':
					commands <- Down{}
				case 'k':
					commands <- Up{}
				case 'l':
					commands <- Enter{}
				case '0':
					fallthrough
				case '1':
					fallthrough
				case '2':
					fallthrough
				case '3':
					fallthrough
				case '4':
					fallthrough
				case '5':
					fallthrough
				case '6':
					fallthrough
				case '7':
					fallthrough
				case '8':
					fallthrough
				case '9':
					commands <- Number{key}
				}
			}
		case *tcell.EventResize:
			s.Sync()
		}
	}
}
