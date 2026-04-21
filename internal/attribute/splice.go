package attribute

import "strings"

func ApplyHunks(oldContent string, hunks []Hunk, newEOFNewline bool) string {
	oldLines, _ := splitLines(oldContent)
	var out []string
	cursor := 1
	for _, h := range hunks {
		if h.OldCount == 0 {
			for i := cursor; i <= h.OldStart; i++ {
				if i-1 < len(oldLines) {
					out = append(out, oldLines[i-1])
				}
			}
			out = append(out, h.NewLines...)
			cursor = h.OldStart + 1
		} else {
			for i := cursor; i < h.OldStart; i++ {
				out = append(out, oldLines[i-1])
			}
			out = append(out, h.NewLines...)
			cursor = h.OldStart + h.OldCount
		}
	}
	for i := cursor; i <= len(oldLines); i++ {
		out = append(out, oldLines[i-1])
	}
	joined := strings.Join(out, "\n")
	if newEOFNewline && len(out) > 0 {
		joined += "\n"
	}
	return joined
}

func splitLines(s string) ([]string, bool) {
	if s == "" {
		return nil, false
	}
	trailing := strings.HasSuffix(s, "\n")
	if trailing {
		s = strings.TrimSuffix(s, "\n")
	}
	return strings.Split(s, "\n"), trailing
}
