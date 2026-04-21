package gitx

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type Git interface {
	Exec(args ...string) ([]byte, error)
	ExecStdin(stdin []byte, args ...string) ([]byte, error)
	Dir() string
}

type Shell struct {
	repoDir string
	env     []string
}

func NewShell(repoDir string) *Shell {
	return &Shell{repoDir: repoDir}
}

func (s *Shell) WithEnv(env []string) *Shell {
	c := *s
	c.env = append([]string(nil), env...)
	return &c
}

func (s *Shell) WithEnvOverride(kv ...string) *Shell {
	c := *s
	c.env = append(append([]string(nil), s.env...), kv...)
	return &c
}

func (s *Shell) Dir() string { return s.repoDir }

func (s *Shell) Exec(args ...string) ([]byte, error) {
	return s.run(nil, args)
}

func (s *Shell) ExecStdin(stdin []byte, args ...string) ([]byte, error) {
	return s.run(stdin, args)
}

func (s *Shell) run(stdin []byte, args []string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = s.repoDir
	if len(s.env) > 0 {
		cmd.Env = append(os.Environ(), s.env...)
	}
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.Bytes(), fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

func HeadSHA(g Git) (string, error) {
	out, err := g.Exec("rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func HeadTreeSHA(g Git) (string, error) {
	out, err := g.Exec("rev-parse", "HEAD^{tree}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func StatusPorcelain(g Git) (string, error) {
	out, err := g.Exec("status", "--porcelain")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func LsFilesTracked(g Git) ([]string, error) {
	out, err := g.Exec("ls-files", "-z")
	if err != nil {
		return nil, err
	}
	raw := bytes.TrimRight(out, "\x00")
	if len(raw) == 0 {
		return nil, nil
	}
	parts := bytes.Split(raw, []byte{0})
	files := make([]string, len(parts))
	for i, p := range parts {
		files[i] = string(p)
	}
	return files, nil
}

func IsBinaryAtHead(g Git, path string) (bool, error) {
	contents, err := g.Exec("show", "HEAD:"+path)
	if err != nil {
		return false, err
	}
	return bytes.IndexByte(contents, 0) >= 0, nil
}

type BlameLine struct {
	Line   int
	Author string
	Email  string
}

func Blame(g Git, path string) ([]BlameLine, error) {
	out, err := g.Exec("blame", "--line-porcelain", "--", path)
	if err != nil {
		return nil, err
	}
	return parseBlamePorcelain(out), nil
}

func parseBlamePorcelain(out []byte) []BlameLine {
	var lines []BlameLine
	var cur *BlameLine
	for _, raw := range strings.Split(string(out), "\n") {
		if isPorcelainHeader(raw) {
			if cur != nil {
				lines = append(lines, *cur)
			}
			fields := strings.Fields(raw)
			b := BlameLine{}
			if len(fields) >= 3 {
				if n, err := strconv.Atoi(fields[2]); err == nil {
					b.Line = n
				}
			}
			cur = &b
			continue
		}
		if cur == nil {
			continue
		}
		switch {
		case strings.HasPrefix(raw, "author "):
			cur.Author = strings.TrimPrefix(raw, "author ")
		case strings.HasPrefix(raw, "author-mail "):
			mail := strings.TrimPrefix(raw, "author-mail ")
			mail = strings.TrimPrefix(mail, "<")
			mail = strings.TrimSuffix(mail, ">")
			cur.Email = mail
		}
	}
	if cur != nil {
		lines = append(lines, *cur)
	}
	return lines
}

func isPorcelainHeader(line string) bool {
	if len(line) < 42 {
		return false
	}
	for i := 0; i < 40; i++ {
		c := line[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	if line[40] != ' ' {
		return false
	}
	rest := strings.Fields(line[41:])
	if len(rest) < 2 {
		return false
	}
	if _, err := strconv.Atoi(rest[0]); err != nil {
		return false
	}
	if _, err := strconv.Atoi(rest[1]); err != nil {
		return false
	}
	return true
}

func DiffUnifiedZero(g Git, oldTree string) ([]byte, error) {
	return g.Exec("diff", "--no-color", "--unified=0", oldTree)
}

func DiffTreesUnifiedZero(g Git, oldTree, newTree string) ([]byte, error) {
	return g.Exec("diff", "--no-color", "--unified=0", oldTree, newTree)
}

func CaptureWorkingTreeSHA(g Git) (string, error) {
	shell, ok := g.(*Shell)
	if !ok {
		out, err := g.Exec("stash", "create")
		if err != nil {
			return "", err
		}
		commit := strings.TrimSpace(string(out))
		if commit == "" {
			return HeadTreeSHA(g)
		}
		treeOut, err := g.Exec("rev-parse", commit+"^{tree}")
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(treeOut)), nil
	}
	tmp, err := os.CreateTemp("", "unblamed-idx-*")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)
	idxShell := shell.WithEnvOverride("GIT_INDEX_FILE=" + tmpPath)
	if _, err := idxShell.Exec("read-tree", "HEAD"); err != nil {
		return "", err
	}
	if _, err := idxShell.Exec("add", "-A"); err != nil {
		return "", err
	}
	out, err := idxShell.Exec("write-tree")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func ResetHard(g Git, ref string) error {
	_, err := g.Exec("reset", "--hard", ref)
	return err
}

func ApplyIndexAndWorkTree(g Git, patch []byte) error {
	_, err := g.ExecStdin(patch, "apply", "--index", "--unidiff-zero", "--allow-empty", "--whitespace=nowarn", "-")
	return err
}

func CommitWithAuthor(g Git, name, email, msg string) (string, error) {
	args := []string{"commit", "--author", fmt.Sprintf("%s <%s>", name, email), "-m", msg, "--allow-empty"}
	_, err := g.Exec(args...)
	if err != nil {
		return "", err
	}
	return HeadSHA(g)
}
