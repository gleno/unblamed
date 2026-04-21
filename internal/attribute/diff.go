package attribute

import (
	"fmt"
	"strconv"
	"strings"
)

type FileKind int

const (
	FileModified FileKind = iota
	FileAdded
	FileDeleted
	FileBinary
)

type FileDiff struct {
	Path     string
	OldPath  string
	Kind     FileKind
	Hunks    []Hunk
	IsBinary bool
	// OldNoEOFMarker: true if the diff carried `\ No newline at end of file` after old-side lines.
	OldNoEOFMarker bool
	// NewNoEOFMarker: true if the diff carried `\ No newline at end of file` after new-side lines.
	NewNoEOFMarker bool
}

// ResolveNewEOFNewline returns whether the target file should end with a newline,
// given the old content's actual EOF state and the diff markers seen.
func (f *FileDiff) ResolveNewEOFNewline(oldEndsWithNewline bool) bool {
	if f.NewNoEOFMarker {
		return false
	}
	if f.OldNoEOFMarker {
		return true
	}
	return oldEndsWithNewline
}

type Hunk struct {
	OldStart int
	OldCount int
	NewStart int
	NewCount int
	OldLines []string
	NewLines []string
}

func ParseDiff(raw []byte) ([]FileDiff, error) {
	text := string(raw)
	lines := strings.Split(text, "\n")
	var out []FileDiff
	var cur *FileDiff
	var hunk *Hunk
	var lastLineKind string

	flushHunk := func() {
		if cur != nil && hunk != nil {
			cur.Hunks = append(cur.Hunks, *hunk)
		}
		hunk = nil
	}
	flushFile := func() {
		flushHunk()
		if cur != nil {
			out = append(out, *cur)
		}
		cur = nil
	}

	for _, l := range lines {
		switch {
		case strings.HasPrefix(l, "diff --git "):
			flushFile()
			a, b, err := parseDiffHeader(l)
			if err != nil {
				return nil, err
			}
			cur = &FileDiff{Path: b, OldPath: a, Kind: FileModified}
		case cur == nil:
			continue
		case strings.HasPrefix(l, "new file mode"):
			cur.Kind = FileAdded
		case strings.HasPrefix(l, "deleted file mode"):
			cur.Kind = FileDeleted
		case strings.HasPrefix(l, "Binary files "):
			cur.IsBinary = true
		case strings.HasPrefix(l, "--- ") || strings.HasPrefix(l, "+++ ") || strings.HasPrefix(l, "index ") || strings.HasPrefix(l, "old mode") || strings.HasPrefix(l, "new mode"):
			continue
		case strings.HasPrefix(l, "@@"):
			flushHunk()
			h, err := parseHunkHeader(l)
			if err != nil {
				return nil, err
			}
			hunk = &h
		case hunk != nil && strings.HasPrefix(l, "-"):
			hunk.OldLines = append(hunk.OldLines, strings.TrimPrefix(l, "-"))
			lastLineKind = "-"
		case hunk != nil && strings.HasPrefix(l, "+"):
			hunk.NewLines = append(hunk.NewLines, strings.TrimPrefix(l, "+"))
			lastLineKind = "+"
		case hunk != nil && strings.HasPrefix(l, `\ No newline at end of file`):
			if cur != nil {
				switch lastLineKind {
				case "+":
					cur.NewNoEOFMarker = true
				case "-":
					cur.OldNoEOFMarker = true
				}
			}
			continue
		}
	}
	flushFile()
	return out, nil
}

func parseDiffHeader(line string) (string, string, error) {
	parts := strings.SplitN(line, " ", 4)
	if len(parts) < 4 {
		return "", "", fmt.Errorf("malformed diff header: %q", line)
	}
	a := strings.TrimPrefix(parts[2], "a/")
	b := strings.TrimPrefix(parts[3], "b/")
	return a, b, nil
}

func parseHunkHeader(line string) (Hunk, error) {
	end := strings.Index(line[2:], "@@")
	if end < 0 {
		return Hunk{}, fmt.Errorf("malformed hunk header: %q", line)
	}
	spec := strings.TrimSpace(line[2 : 2+end])
	parts := strings.Fields(spec)
	if len(parts) != 2 {
		return Hunk{}, fmt.Errorf("malformed hunk spec: %q", spec)
	}
	old, oldCount, err := parseRange(parts[0])
	if err != nil {
		return Hunk{}, err
	}
	new_, newCount, err := parseRange(parts[1])
	if err != nil {
		return Hunk{}, err
	}
	return Hunk{
		OldStart: old,
		OldCount: oldCount,
		NewStart: new_,
		NewCount: newCount,
	}, nil
}

func parseRange(s string) (int, int, error) {
	s = strings.TrimLeft(s, "-+")
	parts := strings.SplitN(s, ",", 2)
	start, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("range start: %q: %w", s, err)
	}
	count := 1
	if len(parts) == 2 {
		count, err = strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, fmt.Errorf("range count: %q: %w", s, err)
		}
	}
	return start, count, nil
}
