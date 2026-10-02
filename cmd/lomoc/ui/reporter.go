package ui

import (
	"fmt"

	"bitbucket.org/lomoware/lomo-backend/common/scan"
	"github.com/mattn/go-runewidth"
)

// Line represents row of text in folder UI contains info about subfile
type Line struct {
	Text     []rune
	IsMarked bool
}

// ReportFolder converts all subfiles into UI lines
func ReportFolder(folder *scan.File, markedFiles map[*scan.File]struct{}) []Line {
	report := make([]Line, len(folder.Children))
	for index, file := range folder.Children {
		name := file.Name
		if len(file.Children) > 0 {
			if file.Children[0].IsFakeFile() {
				name = name + " !"
			} else {
				name = name + "/"
			}
		}
		marking := " "
		_, isMarked := markedFiles[file]
		if isMarked {
			marking = "*"
		}
		report[index] = Line{
			Text:     appendRune([]rune(fmt.Sprintf("%s %s", marking, name))),
			IsMarked: isMarked,
		}
	}
	return report
}

func appendRune(input []rune) []rune {
	output := []rune{}
	for _, ch := range input {
		output = append(output, ch)
		for i := 1; i < runewidth.RuneWidth(ch); i++ {
			output = append(output, ' ')
		}
	}
	return output
}
