package mergeinfo

import (
	"fmt"
	"strings"
)

// Conflict marker prefixes as git writes them at the start of a line.
const (
	oursMarker      = "<<<<<<< "
	baseMarker      = "||||||| "
	separatorMarker = "======="
	theirsMarker    = ">>>>>>> "
)

// parseSection says which part of a conflict the parser is currently reading.
type parseSection int

const (
	outsideHunk parseSection = iota
	inOurs
	inBase
	inTheirs
)

// ParseHunks reads conflict markers from file content; exported so the parsing can be tested without git.
func ParseHunks(content string) ([]Hunk, error) {
	// SplitAfter keeps each line's "\n" or "\r\n" so the sides keep their original endings.
	lines := strings.SplitAfter(content, "\n")
	hunks := []Hunk{}
	section := outsideHunk
	var current Hunk
	var ours, base, theirs strings.Builder
	for index, line := range lines {
		lineNumber := index + 1
		switch {
		case strings.HasPrefix(line, oursMarker):
			if section != outsideHunk {
				return nil, fmt.Errorf("mergeinfo: line %d: <<<<<<< inside the conflict that starts at line %d", lineNumber, current.Start)
			}
			section = inOurs
			current = Hunk{Start: lineNumber}
			ours.Reset()
			base.Reset()
			theirs.Reset()
		case strings.HasPrefix(line, baseMarker):
			if section != inOurs {
				return nil, fmt.Errorf("mergeinfo: line %d: ||||||| out of order", lineNumber)
			}
			section = inBase
		case strings.HasPrefix(line, separatorMarker):
			if section != inOurs && section != inBase {
				return nil, fmt.Errorf("mergeinfo: line %d: ======= out of order", lineNumber)
			}
			section = inTheirs
		case strings.HasPrefix(line, theirsMarker):
			if section != inTheirs {
				return nil, fmt.Errorf("mergeinfo: line %d: >>>>>>> out of order", lineNumber)
			}
			current.End = lineNumber
			current.Ours = ours.String()
			current.Base = base.String()
			current.Theirs = theirs.String()
			current.OursDeleted = current.Ours == ""
			current.TheirsDeleted = current.Theirs == ""
			hunks = append(hunks, current)
			section = outsideHunk
		default:
			switch section {
			case inOurs:
				ours.WriteString(line)
			case inBase:
				base.WriteString(line)
			case inTheirs:
				theirs.WriteString(line)
			}
		}
	}
	if section != outsideHunk {
		return nil, fmt.Errorf("mergeinfo: line %d: <<<<<<< without matching >>>>>>>", current.Start)
	}
	return hunks, nil
}
