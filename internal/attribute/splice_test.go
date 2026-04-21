package attribute

import "testing"

func TestApplyHunks_Replace(t *testing.T) {
	got := ApplyHunks("a\nb\nc\n", []Hunk{{OldStart: 2, OldCount: 1, NewStart: 2, NewCount: 1, NewLines: []string{"B"}}})
	if got != "a\nB\nc\n" {
		t.Errorf("got %q", got)
	}
}

func TestApplyHunks_InsertMiddle(t *testing.T) {
	got := ApplyHunks("a\nb\n", []Hunk{{OldStart: 1, OldCount: 0, NewStart: 2, NewCount: 1, NewLines: []string{"X"}}})
	if got != "a\nX\nb\n" {
		t.Errorf("got %q", got)
	}
}

func TestApplyHunks_InsertTop(t *testing.T) {
	got := ApplyHunks("a\nb\n", []Hunk{{OldStart: 0, OldCount: 0, NewStart: 1, NewCount: 1, NewLines: []string{"X"}}})
	if got != "X\na\nb\n" {
		t.Errorf("got %q", got)
	}
}

func TestApplyHunks_DeleteTail(t *testing.T) {
	got := ApplyHunks("a\nb\nc\n", []Hunk{{OldStart: 3, OldCount: 1, NewStart: 3, NewCount: 0}})
	if got != "a\nb\n" {
		t.Errorf("got %q", got)
	}
}

func TestApplyHunks_JoinTwoToOne(t *testing.T) {
	got := ApplyHunks("aaa\nbbb\n", []Hunk{{OldStart: 1, OldCount: 2, NewStart: 1, NewCount: 1, NewLines: []string{"aaa bbb"}}})
	if got != "aaa bbb\n" {
		t.Errorf("got %q", got)
	}
}

func TestApplyHunks_SplitOneToTwo(t *testing.T) {
	got := ApplyHunks("first second\n", []Hunk{{OldStart: 1, OldCount: 1, NewStart: 1, NewCount: 2, NewLines: []string{"first", "second"}}})
	if got != "first\nsecond\n" {
		t.Errorf("got %q", got)
	}
}

func TestApplyHunks_TwoHunksPreserveSeparator(t *testing.T) {
	src := "a\nb\nSEP\nc\nd\n"
	hunks := []Hunk{
		{OldStart: 1, OldCount: 2, NewStart: 1, NewCount: 2, NewLines: []string{"A", "B"}},
		{OldStart: 4, OldCount: 2, NewStart: 4, NewCount: 2, NewLines: []string{"C", "D"}},
	}
	if got := ApplyHunks(src, hunks); got != "A\nB\nSEP\nC\nD\n" {
		t.Errorf("got %q", got)
	}
}
