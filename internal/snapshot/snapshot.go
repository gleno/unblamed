package snapshot

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gleno/unblamed/internal/gitx"
)

type LineAuthor struct {
	Line   int    `json:"line"`
	Author string `json:"author"`
	Email  string `json:"email"`
}

type Snapshot struct {
	HeadSHA   string                  `json:"headSha"`
	TreeSHA   string                  `json:"treeSha"`
	Files     map[string][]LineAuthor `json:"files"`
	Fallback  Identity                `json:"fallback"`
	CreatedAt string                  `json:"createdAt"`
}

type Identity struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

func Path(g gitx.Git) string {
	return filepath.Join(g.Dir(), ".git", "unblamed", "snapshot.json")
}

func Dir(g gitx.Git) string {
	return filepath.Join(g.Dir(), ".git", "unblamed")
}

func Exists(g gitx.Git) bool {
	_, err := os.Stat(Path(g))
	return err == nil
}

func Load(g gitx.Git) (*Snapshot, error) {
	b, err := os.ReadFile(Path(g))
	if err != nil {
		return nil, err
	}
	var s Snapshot
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("parse snapshot: %w", err)
	}
	return &s, nil
}

func Save(g gitx.Git, s *Snapshot) error {
	if err := os.MkdirAll(Dir(g), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(Path(g), b, 0o644)
}

func Delete(g gitx.Git) error {
	err := os.Remove(Path(g))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (s *Snapshot) AuthorAt(file string, line int) (Identity, bool) {
	entries, ok := s.Files[file]
	if !ok {
		return Identity{}, false
	}
	for _, e := range entries {
		if e.Line == line {
			return Identity{Name: e.Author, Email: e.Email}, true
		}
	}
	return Identity{}, false
}

func (s *Snapshot) SortedFiles() []string {
	out := make([]string, 0, len(s.Files))
	for k := range s.Files {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func FallbackFromConfig(g gitx.Git) (Identity, error) {
	name, err := g.Exec("config", "user.name")
	if err != nil {
		return Identity{}, fmt.Errorf("read user.name: %w", err)
	}
	email, err := g.Exec("config", "user.email")
	if err != nil {
		return Identity{}, fmt.Errorf("read user.email: %w", err)
	}
	n := strings.TrimSpace(string(name))
	e := strings.TrimSpace(string(email))
	if n == "" || e == "" {
		return Identity{}, fmt.Errorf("git user.name / user.email not set")
	}
	return Identity{Name: n, Email: e}, nil
}
