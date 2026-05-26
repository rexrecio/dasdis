package store

import (
	"math"
	"testing"
)

// --- generic ---

func TestDel(t *testing.T) {
	s := New()
	s.Set("a", "1")
	s.Set("b", "2")
	if n := s.Del([]string{"a", "c"}); n != 1 {
		t.Fatalf("Del returned %d, want 1", n)
	}
	if _, ok, _ := s.Get("a"); ok {
		t.Fatal("key 'a' should be deleted")
	}
	if _, ok, _ := s.Get("b"); !ok {
		t.Fatal("key 'b' should still exist")
	}
}

func TestExists(t *testing.T) {
	s := New()
	s.Set("x", "v")
	if n := s.Exists([]string{"x", "x", "missing"}); n != 2 {
		t.Fatalf("Exists returned %d, want 2", n)
	}
}

// --- strings ---

func TestGetSet(t *testing.T) {
	s := New()
	_, ok, _ := s.Get("missing")
	if ok {
		t.Fatal("expected missing key")
	}
	s.Set("k", "hello")
	v, ok, err := s.Get("k")
	if err != nil || !ok || v != "hello" {
		t.Fatalf("Get returned (%q, %v, %v)", v, ok, err)
	}
}

func TestGet_WrongType(t *testing.T) {
	s := New()
	s.LPush("mylist", []string{"a"})
	_, _, err := s.Get("mylist")
	if err != ErrWrongType {
		t.Fatalf("expected ErrWrongType, got %v", err)
	}
}

func TestIncrBy(t *testing.T) {
	s := New()
	n, err := s.IncrBy("counter", 5)
	if err != nil || n != 5 {
		t.Fatalf("IncrBy: got (%d, %v)", n, err)
	}
	n, err = s.IncrBy("counter", -2)
	if err != nil || n != 3 {
		t.Fatalf("IncrBy: got (%d, %v)", n, err)
	}
}

func TestIncrBy_NotInt(t *testing.T) {
	s := New()
	s.Set("k", "notanumber")
	_, err := s.IncrBy("k", 1)
	if err != ErrNotInt {
		t.Fatalf("expected ErrNotInt, got %v", err)
	}
}

func TestIncrBy_WrongType(t *testing.T) {
	s := New()
	s.LPush("mylist", []string{"a"})
	_, err := s.IncrBy("mylist", 1)
	if err != ErrWrongType {
		t.Fatalf("expected ErrWrongType, got %v", err)
	}
}

// --- lists ---

func TestLPushLPop(t *testing.T) {
	s := New()
	n, _ := s.LPush("l", []string{"a", "b", "c"})
	if n != 3 {
		t.Fatalf("LPush returned %d, want 3", n)
	}
	// LPush prepends each in order, so head is "c"
	v, ok, _ := s.LPop("l")
	if !ok || v != "c" {
		t.Fatalf("LPop returned (%q, %v)", v, ok)
	}
}

func TestRPushRPop(t *testing.T) {
	s := New()
	s.RPush("l", []string{"a", "b", "c"})
	v, ok, _ := s.RPop("l")
	if !ok || v != "c" {
		t.Fatalf("RPop returned (%q, %v)", v, ok)
	}
}

func TestLPop_Missing(t *testing.T) {
	s := New()
	_, ok, err := s.LPop("missing")
	if ok || err != nil {
		t.Fatalf("expected (false, nil), got (%v, %v)", ok, err)
	}
}

func TestLLen(t *testing.T) {
	s := New()
	s.RPush("l", []string{"a", "b", "c"})
	n, _ := s.LLen("l")
	if n != 3 {
		t.Fatalf("LLen = %d, want 3", n)
	}
}

func TestLRange(t *testing.T) {
	s := New()
	s.RPush("l", []string{"a", "b", "c", "d"})

	got, _ := s.LRange("l", 0, -1)
	if len(got) != 4 || got[0] != "a" || got[3] != "d" {
		t.Fatalf("LRange(0,-1) = %v", got)
	}

	got, _ = s.LRange("l", 1, 2)
	if len(got) != 2 || got[0] != "b" || got[1] != "c" {
		t.Fatalf("LRange(1,2) = %v", got)
	}
}

func TestList_DeletesKeyWhenEmpty(t *testing.T) {
	s := New()
	s.LPush("l", []string{"a"})
	s.LPop("l")
	if n := s.Exists([]string{"l"}); n != 0 {
		t.Fatal("key should be removed after list is empty")
	}
}

func TestList_WrongType(t *testing.T) {
	s := New()
	s.Set("str", "v")
	_, err := s.LPush("str", []string{"x"})
	if err != ErrWrongType {
		t.Fatalf("expected ErrWrongType, got %v", err)
	}
}

// --- clampRange ---

func TestClampRange(t *testing.T) {
	cases := []struct{ start, stop, n, lo, hi int64 }{
		{0, -1, 5, 0, 4},
		{0, 10, 5, 0, 4},
		{-2, -1, 5, 3, 4},
		{5, 10, 5, 5, 4}, // start past end → lo > hi (empty)
		{0, -1, 0, 0, -1}, // empty slice
	}
	for _, c := range cases {
		lo, hi := clampRange(c.start, c.stop, c.n)
		if lo != c.lo || hi != c.hi {
			t.Errorf("clampRange(%d,%d,%d) = (%d,%d), want (%d,%d)",
				c.start, c.stop, c.n, lo, hi, c.lo, c.hi)
		}
	}
}

// --- sorted sets ---

func TestZAddZScore(t *testing.T) {
	s := New()
	n, _ := s.ZAdd("z", []ZAddPair{{1.0, "a"}, {2.0, "b"}})
	if n != 2 {
		t.Fatalf("ZAdd returned %d, want 2", n)
	}
	score, ok, _ := s.ZScore("z", "a")
	if !ok || score != 1.0 {
		t.Fatalf("ZScore = (%v, %v)", score, ok)
	}
}

func TestZAdd_UpdateScore(t *testing.T) {
	s := New()
	s.ZAdd("z", []ZAddPair{{1.0, "a"}})
	n, _ := s.ZAdd("z", []ZAddPair{{99.0, "a"}})
	if n != 0 {
		t.Fatalf("ZAdd update should return 0 added, got %d", n)
	}
	score, _, _ := s.ZScore("z", "a")
	if score != 99.0 {
		t.Fatalf("score not updated: got %v", score)
	}
}

func TestZAdd_SameScore(t *testing.T) {
	s := New()
	s.ZAdd("z", []ZAddPair{{1.0, "a"}})
	n, _ := s.ZAdd("z", []ZAddPair{{1.0, "a"}}) // same score, no-op
	if n != 0 {
		t.Fatalf("ZAdd same score should return 0, got %d", n)
	}
}

func TestZRem(t *testing.T) {
	s := New()
	s.ZAdd("z", []ZAddPair{{1.0, "a"}, {2.0, "b"}})
	n, _ := s.ZRem("z", []string{"a", "missing"})
	if n != 1 {
		t.Fatalf("ZRem returned %d, want 1", n)
	}
	_, ok, _ := s.ZScore("z", "a")
	if ok {
		t.Fatal("member 'a' should be removed")
	}
}

func TestZRange(t *testing.T) {
	s := New()
	s.ZAdd("z", []ZAddPair{{3.0, "c"}, {1.0, "a"}, {2.0, "b"}})
	got, _ := s.ZRange("z", 0, -1)
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("ZRange = %v, want members %v", got, want)
	}
	for i, e := range got {
		if e.Member != want[i] {
			t.Fatalf("ZRange[%d].Member = %q, want %q", i, e.Member, want[i])
		}
	}
}

func TestZRange_Slice(t *testing.T) {
	s := New()
	s.ZAdd("z", []ZAddPair{{1.0, "a"}, {2.0, "b"}, {3.0, "c"}, {4.0, "d"}})
	got, _ := s.ZRange("z", 1, 2)
	if len(got) != 2 || got[0].Member != "b" || got[1].Member != "c" {
		t.Fatalf("ZRange(1,2) = %v", got)
	}
}

func TestEncodeKey_Ordering(t *testing.T) {
	scores := []float64{math.Inf(-1), -1.0, 0.0, 1.0, math.Inf(1)}
	prev := encodeKey(scores[0], "")
	for _, sc := range scores[1:] {
		cur := encodeKey(sc, "")
		if cur <= prev {
			t.Fatalf("encodeKey(%v) <= encodeKey(prev): ordering broken", sc)
		}
		prev = cur
	}
}

func TestZSet_WrongType(t *testing.T) {
	s := New()
	s.Set("str", "v")
	_, err := s.ZAdd("str", []ZAddPair{{1.0, "a"}})
	if err != ErrWrongType {
		t.Fatalf("expected ErrWrongType, got %v", err)
	}
}

func TestZSet_DeletesKeyWhenEmpty(t *testing.T) {
	s := New()
	s.ZAdd("z", []ZAddPair{{1.0, "a"}})
	s.ZRem("z", []string{"a"})
	if n := s.Exists([]string{"z"}); n != 0 {
		t.Fatal("key should be removed after zset is empty")
	}
}
