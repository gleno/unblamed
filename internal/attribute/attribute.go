package attribute

import (
	"sort"
	"strings"

	"github.com/gleno/unblamed/internal/snapshot"
)

type AuthorKey struct {
	Name  string
	Email string
}

type HunkAssignment struct {
	File string
	Hunk Hunk
}

type FileOp struct {
	Path   string
	OldPath string
	Kind   FileKind
}

type Plan struct {
	Order   []AuthorKey
	Hunks   map[AuthorKey][]HunkAssignment
	FileOps map[AuthorKey][]FileOp
	Fallback AuthorKey
}

func BuildPlan(snap *snapshot.Snapshot, files []FileDiff, fallback snapshot.Identity) Plan {
	fallbackKey := AuthorKey{Name: fallback.Name, Email: fallback.Email}
	p := Plan{
		Hunks:    map[AuthorKey][]HunkAssignment{},
		FileOps:  map[AuthorKey][]FileOp{},
		Fallback: fallbackKey,
	}
	seen := map[AuthorKey]struct{}{}
	add := func(k AuthorKey) {
		if _, ok := seen[k]; !ok {
			seen[k] = struct{}{}
		}
	}
	for _, f := range files {
		if f.IsBinary || f.Kind == FileAdded || f.Kind == FileDeleted {
			p.FileOps[fallbackKey] = append(p.FileOps[fallbackKey], FileOp{Path: f.Path, OldPath: f.OldPath, Kind: f.Kind})
			add(fallbackKey)
			continue
		}
		for _, h := range f.Hunks {
			owner := hunkOwner(snap, f.Path, h, fallback)
			p.Hunks[owner] = append(p.Hunks[owner], HunkAssignment{File: f.Path, Hunk: h})
			add(owner)
		}
	}
	p.Order = sortedAuthors(seen, fallbackKey)
	return p
}

func hunkOwner(snap *snapshot.Snapshot, path string, h Hunk, fallback snapshot.Identity) AuthorKey {
	if h.OldCount == 0 {
		if h.OldStart <= 0 {
			return AuthorKey{Name: fallback.Name, Email: fallback.Email}
		}
		id, ok := snap.AuthorAt(path, h.OldStart)
		if !ok {
			return AuthorKey{Name: fallback.Name, Email: fallback.Email}
		}
		return AuthorKey{Name: id.Name, Email: id.Email}
	}
	type tally struct {
		count int
		first int
		key   AuthorKey
	}
	tallies := map[AuthorKey]*tally{}
	for i := 0; i < h.OldCount; i++ {
		line := h.OldStart + i
		id, ok := snap.AuthorAt(path, line)
		var key AuthorKey
		if !ok {
			key = AuthorKey{Name: fallback.Name, Email: fallback.Email}
		} else {
			key = AuthorKey{Name: id.Name, Email: id.Email}
		}
		content := ""
		if i < len(h.OldLines) {
			content = h.OldLines[i]
		}
		weight := len(content)
		if weight == 0 {
			weight = 1
		}
		t, ok := tallies[key]
		if !ok {
			t = &tally{first: i, key: key}
			tallies[key] = t
		}
		t.count += weight
	}
	var best *tally
	for _, t := range tallies {
		if best == nil || t.count > best.count || (t.count == best.count && t.first < best.first) {
			best = t
		}
	}
	return best.key
}

func sortedAuthors(seen map[AuthorKey]struct{}, fallback AuthorKey) []AuthorKey {
	_, hasFallback := seen[fallback]
	out := make([]AuthorKey, 0, len(seen))
	for k := range seen {
		if k == fallback {
			continue
		}
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Email) < strings.ToLower(out[j].Email)
	})
	if hasFallback {
		out = append(out, fallback)
	}
	return out
}

func (p *Plan) FilesTouchedCumulative(upToIndex int) map[string]struct{} {
	out := map[string]struct{}{}
	for i := 0; i <= upToIndex && i < len(p.Order); i++ {
		k := p.Order[i]
		for _, h := range p.Hunks[k] {
			out[h.File] = struct{}{}
		}
		for _, op := range p.FileOps[k] {
			out[op.Path] = struct{}{}
			if op.OldPath != "" {
				out[op.OldPath] = struct{}{}
			}
		}
	}
	return out
}

func (p *Plan) HunksForFileUpTo(file string, upToIndex int) []Hunk {
	var hs []Hunk
	for i := 0; i <= upToIndex && i < len(p.Order); i++ {
		k := p.Order[i]
		for _, h := range p.Hunks[k] {
			if h.File == file {
				hs = append(hs, h.Hunk)
			}
		}
	}
	sort.Slice(hs, func(i, j int) bool { return hs[i].OldStart < hs[j].OldStart })
	return hs
}

func (p *Plan) FileOpsUpTo(upToIndex int) []FileOp {
	var ops []FileOp
	for i := 0; i <= upToIndex && i < len(p.Order); i++ {
		ops = append(ops, p.FileOps[p.Order[i]]...)
	}
	return ops
}
