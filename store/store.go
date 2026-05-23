// Package store is the in-memory key-value store backing dasredis.
// Strings are kept in a plain map; lists use dasgo/linkedlist; sorted
// sets use dasgo/avl with a lex-sortable composite key. There is no
// persistence — all data is lost when the process exits.
package store

import (
	"encoding/binary"
	"errors"
	"math"
	"strconv"
	"sync"

	"github.com/rexrecio/dasgo/avl"
	"github.com/rexrecio/dasgo/linkedlist"
)

var (
	ErrWrongType = errors.New("WRONGTYPE Operation against a key holding the wrong kind of value")
	ErrNotInt    = errors.New("ERR value is not an integer or out of range")
)

type kind int

const (
	kindNone kind = iota
	kindString
	kindList
	kindZSet
)

type entry struct {
	kind   kind
	str    string
	list   *linkedlist.SinglyLinkedList[string]
	zset   *zset
}

// Store is a goroutine-safe in-memory store. All operations take the
// store-level mutex; the dasgo containers' internal locks are not relied on.
type Store struct {
	mu   sync.Mutex
	data map[string]*entry
}

func New() *Store {
	return &Store{data: make(map[string]*entry)}
}

// --- generic ---

func (s *Store) Del(keys []string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	var n int64
	for _, k := range keys {
		if _, ok := s.data[k]; ok {
			delete(s.data, k)
			n++
		}
	}
	return n
}

func (s *Store) Exists(keys []string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	var n int64
	for _, k := range keys {
		if _, ok := s.data[k]; ok {
			n++
		}
	}
	return n
}

// --- strings ---

func (s *Store) Get(key string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.data[key]
	if !ok {
		return "", false, nil
	}
	if e.kind != kindString {
		return "", false, ErrWrongType
	}
	return e.str, true, nil
}

func (s *Store) Set(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = &entry{kind: kindString, str: value}
}

func (s *Store) IncrBy(key string, delta int64) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.data[key]
	var cur int64
	if ok {
		if e.kind != kindString {
			return 0, ErrWrongType
		}
		n, err := strconv.ParseInt(e.str, 10, 64)
		if err != nil {
			return 0, ErrNotInt
		}
		cur = n
	}
	cur += delta
	s.data[key] = &entry{kind: kindString, str: strconv.FormatInt(cur, 10)}
	return cur, nil
}

// --- lists ---

func (s *Store) listFor(key string, create bool) (*linkedlist.SinglyLinkedList[string], error) {
	e, ok := s.data[key]
	if !ok {
		if !create {
			return nil, nil
		}
		l := linkedlist.New[string]()
		s.data[key] = &entry{kind: kindList, list: l}
		return l, nil
	}
	if e.kind != kindList {
		return nil, ErrWrongType
	}
	return e.list, nil
}

func (s *Store) LPush(key string, values []string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, err := s.listFor(key, true)
	if err != nil {
		return 0, err
	}
	for _, v := range values {
		l.Prepend(v)
	}
	return int64(l.Len()), nil
}

func (s *Store) RPush(key string, values []string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, err := s.listFor(key, true)
	if err != nil {
		return 0, err
	}
	for _, v := range values {
		l.Append(v)
	}
	return int64(l.Len()), nil
}

func (s *Store) LPop(key string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, err := s.listFor(key, false)
	if err != nil || l == nil {
		return "", false, err
	}
	v, ok := l.PopFront()
	if l.IsEmpty() {
		delete(s.data, key)
	}
	return v, ok, nil
}

func (s *Store) RPop(key string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, err := s.listFor(key, false)
	if err != nil || l == nil {
		return "", false, err
	}
	// SinglyLinkedList has no O(1) tail removal; delete the node at the last
	// index using DeleteFunc, which removes the first match it walks past.
	target := l.Len() - 1
	if target < 0 {
		return "", false, nil
	}
	var popped string
	idx := 0
	l.DeleteFunc(func(v string) bool {
		if idx == target {
			popped = v
			return true
		}
		idx++
		return false
	})
	if l.IsEmpty() {
		delete(s.data, key)
	}
	return popped, true, nil
}

func (s *Store) LLen(key string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, err := s.listFor(key, false)
	if err != nil || l == nil {
		return 0, err
	}
	return int64(l.Len()), nil
}

func (s *Store) LRange(key string, start, stop int64) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, err := s.listFor(key, false)
	if err != nil || l == nil {
		return nil, err
	}
	values := l.Values()
	lo, hi := clampRange(start, stop, int64(len(values)))
	if lo > hi {
		return []string{}, nil
	}
	return values[lo : hi+1], nil
}

// clampRange normalises Redis-style inclusive [start,stop] indices (negatives
// count from the end) against a slice of length n. Returns the empty range
// (lo > hi) when the request resolves to nothing.
func clampRange(start, stop, n int64) (int64, int64) {
	if n == 0 {
		return 0, -1
	}
	if start < 0 {
		start += n
	}
	if stop < 0 {
		stop += n
	}
	if start < 0 {
		start = 0
	}
	if stop >= n {
		stop = n - 1
	}
	return start, stop
}

// --- sorted sets ---

type zset struct {
	scores map[string]float64
	tree   *avl.AVLTree[string]
}

func newZSet() *zset {
	return &zset{
		scores: make(map[string]float64),
		tree:   avl.New[string](),
	}
}

// encodeKey produces a string that, sorted lexicographically, orders entries
// by (score asc, member asc) — matching Redis semantics for the sorted set.
// The score is rendered as 8 bytes of order-preserving IEEE 754, followed by
// a NUL separator, followed by the raw member bytes.
func encodeKey(score float64, member string) string {
	bits := math.Float64bits(score)
	if bits&(1<<63) != 0 {
		// negative: flip all bits so larger magnitudes sort smaller
		bits = ^bits
	} else {
		// non-negative: flip sign bit so positives sort above negatives
		bits ^= 1 << 63
	}
	b := make([]byte, 9+len(member))
	binary.BigEndian.PutUint64(b[:8], bits)
	b[8] = 0
	copy(b[9:], member)
	return string(b)
}

func decodeMember(key string) string {
	return key[9:]
}

func (s *Store) zsetFor(key string, create bool) (*zset, error) {
	e, ok := s.data[key]
	if !ok {
		if !create {
			return nil, nil
		}
		z := newZSet()
		s.data[key] = &entry{kind: kindZSet, zset: z}
		return z, nil
	}
	if e.kind != kindZSet {
		return nil, ErrWrongType
	}
	return e.zset, nil
}

// ZAddPair is one (score, member) pair for ZADD.
type ZAddPair struct {
	Score  float64
	Member string
}

// ZAdd inserts/updates members and returns the count of newly added members.
func (s *Store) ZAdd(key string, pairs []ZAddPair) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	z, err := s.zsetFor(key, true)
	if err != nil {
		return 0, err
	}
	var added int64
	for _, p := range pairs {
		if oldScore, exists := z.scores[p.Member]; exists {
			if oldScore == p.Score {
				continue
			}
			z.tree.Delete(encodeKey(oldScore, p.Member))
		} else {
			added++
		}
		z.scores[p.Member] = p.Score
		z.tree.Insert(encodeKey(p.Score, p.Member))
	}
	return added, nil
}

func (s *Store) ZScore(key, member string) (float64, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	z, err := s.zsetFor(key, false)
	if err != nil || z == nil {
		return 0, false, err
	}
	score, ok := z.scores[member]
	return score, ok, nil
}

func (s *Store) ZRem(key string, members []string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	z, err := s.zsetFor(key, false)
	if err != nil || z == nil {
		return 0, err
	}
	var removed int64
	for _, m := range members {
		score, ok := z.scores[m]
		if !ok {
			continue
		}
		z.tree.Delete(encodeKey(score, m))
		delete(z.scores, m)
		removed++
	}
	if len(z.scores) == 0 {
		delete(s.data, key)
	}
	return removed, nil
}

// ZRangeEntry is one entry returned by ZRange.
type ZRangeEntry struct {
	Member string
	Score  float64
}

// ZRange returns members in score order over the inclusive index range
// [start, stop] (negatives count from the end). Scores are filled in.
func (s *Store) ZRange(key string, start, stop int64) ([]ZRangeEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	z, err := s.zsetFor(key, false)
	if err != nil || z == nil {
		return nil, err
	}
	n := int64(len(z.scores))
	lo, hi := clampRange(start, stop, n)
	if lo > hi {
		return []ZRangeEntry{}, nil
	}
	out := make([]ZRangeEntry, 0, hi-lo+1)
	idx := int64(0)
	z.tree.ForEach(func(key string) bool {
		if idx > hi {
			return false
		}
		if idx >= lo {
			m := decodeMember(key)
			out = append(out, ZRangeEntry{Member: m, Score: z.scores[m]})
		}
		idx++
		return true
	})
	return out, nil
}
