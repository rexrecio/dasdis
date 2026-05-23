package server

import (
	"strconv"
	"strings"

	"github.com/rexrecio/dasdis/resp"
	"github.com/rexrecio/dasdis/store"
)

// --- connection ---

func (s *Server) cmdPing(w *resp.Writer, args []string) {
	switch len(args) {
	case 1:
		_ = w.SimpleString("PONG")
	case 2:
		_ = w.BulkString(args[1])
	default:
		wrongArgs(w, args[0])
	}
}

func (s *Server) cmdEcho(w *resp.Writer, args []string) {
	if len(args) != 2 {
		wrongArgs(w, args[0])
		return
	}
	_ = w.BulkString(args[1])
}

func (s *Server) cmdHello(w *resp.Writer, args []string) {
	// Only RESP2 is supported. If a protocol is requested, reject anything
	// other than 2; otherwise reply with a minimal handshake map encoded as
	// the RESP2 flat array Redis uses for this case.
	if len(args) >= 2 {
		if v, err := strconv.Atoi(args[1]); err == nil && v != 2 {
			_ = w.Error("NOPROTO unsupported protocol version")
			return
		}
	}
	pairs := []string{
		"server", "dasdis",
		"version", "0.1.0",
		"proto", "2",
		"mode", "standalone",
	}
	_ = w.ArrayHeader(len(pairs))
	for i, v := range pairs {
		if i%2 == 1 && (pairs[i-1] == "proto") {
			_ = w.Integer(2)
			continue
		}
		_ = w.BulkString(v)
	}
}

// --- generic keys ---

func (s *Server) cmdDel(w *resp.Writer, args []string) {
	if len(args) < 2 {
		wrongArgs(w, args[0])
		return
	}
	_ = w.Integer(s.store.Del(args[1:]))
}

func (s *Server) cmdExists(w *resp.Writer, args []string) {
	if len(args) < 2 {
		wrongArgs(w, args[0])
		return
	}
	_ = w.Integer(s.store.Exists(args[1:]))
}

// --- strings ---

func (s *Server) cmdGet(w *resp.Writer, args []string) {
	if len(args) != 2 {
		wrongArgs(w, args[0])
		return
	}
	v, ok, err := s.store.Get(args[1])
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if !ok {
		_ = w.NullBulk()
		return
	}
	_ = w.BulkString(v)
}

func (s *Server) cmdSet(w *resp.Writer, args []string) {
	// Accept "SET key value" plus options we silently ignore (EX, PX, NX, XX).
	// TTL is not stored — this server has no expiration support.
	if len(args) < 3 {
		wrongArgs(w, args[0])
		return
	}
	s.store.Set(args[1], args[2])
	_ = w.SimpleString("OK")
}

func (s *Server) cmdIncrBy(w *resp.Writer, name string, rest []string, delta int64) {
	if len(rest) != 1 {
		wrongArgs(w, name)
		return
	}
	n, err := s.store.IncrBy(rest[0], delta)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	_ = w.Integer(n)
}

// --- lists ---

func (s *Server) cmdPush(w *resp.Writer, args []string, left bool) {
	if len(args) < 3 {
		wrongArgs(w, args[0])
		return
	}
	var (
		n   int64
		err error
	)
	if left {
		n, err = s.store.LPush(args[1], args[2:])
	} else {
		n, err = s.store.RPush(args[1], args[2:])
	}
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	_ = w.Integer(n)
}

func (s *Server) cmdPop(w *resp.Writer, args []string, left bool) {
	if len(args) != 2 {
		wrongArgs(w, args[0])
		return
	}
	var (
		v   string
		ok  bool
		err error
	)
	if left {
		v, ok, err = s.store.LPop(args[1])
	} else {
		v, ok, err = s.store.RPop(args[1])
	}
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if !ok {
		_ = w.NullBulk()
		return
	}
	_ = w.BulkString(v)
}

func (s *Server) cmdLLen(w *resp.Writer, args []string) {
	if len(args) != 2 {
		wrongArgs(w, args[0])
		return
	}
	n, err := s.store.LLen(args[1])
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	_ = w.Integer(n)
}

func (s *Server) cmdLRange(w *resp.Writer, args []string) {
	if len(args) != 4 {
		wrongArgs(w, args[0])
		return
	}
	start, err := strconv.ParseInt(args[2], 10, 64)
	if err != nil {
		_ = w.Error("ERR value is not an integer or out of range")
		return
	}
	stop, err := strconv.ParseInt(args[3], 10, 64)
	if err != nil {
		_ = w.Error("ERR value is not an integer or out of range")
		return
	}
	items, err := s.store.LRange(args[1], start, stop)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	_ = w.StringArray(items)
}

// --- sorted sets ---

func (s *Server) cmdZAdd(w *resp.Writer, args []string) {
	// ZADD key score member [score member ...]
	if len(args) < 4 || (len(args)-2)%2 != 0 {
		wrongArgs(w, args[0])
		return
	}
	pairs := make([]store.ZAddPair, 0, (len(args)-2)/2)
	for i := 2; i < len(args); i += 2 {
		score, err := strconv.ParseFloat(args[i], 64)
		if err != nil {
			_ = w.Error("ERR value is not a valid float")
			return
		}
		pairs = append(pairs, store.ZAddPair{Score: score, Member: args[i+1]})
	}
	n, err := s.store.ZAdd(args[1], pairs)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	_ = w.Integer(n)
}

func (s *Server) cmdZRange(w *resp.Writer, args []string) {
	// ZRANGE key start stop [WITHSCORES]
	if len(args) != 4 && len(args) != 5 {
		wrongArgs(w, args[0])
		return
	}
	withScores := false
	if len(args) == 5 {
		if !strings.EqualFold(args[4], "WITHSCORES") {
			_ = w.Error("ERR syntax error")
			return
		}
		withScores = true
	}
	start, err := strconv.ParseInt(args[2], 10, 64)
	if err != nil {
		_ = w.Error("ERR value is not an integer or out of range")
		return
	}
	stop, err := strconv.ParseInt(args[3], 10, 64)
	if err != nil {
		_ = w.Error("ERR value is not an integer or out of range")
		return
	}
	entries, err := s.store.ZRange(args[1], start, stop)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	count := len(entries)
	if withScores {
		count *= 2
	}
	_ = w.ArrayHeader(count)
	for _, e := range entries {
		_ = w.BulkString(e.Member)
		if withScores {
			_ = w.BulkString(formatFloat(e.Score))
		}
	}
}

func (s *Server) cmdZScore(w *resp.Writer, args []string) {
	if len(args) != 3 {
		wrongArgs(w, args[0])
		return
	}
	score, ok, err := s.store.ZScore(args[1], args[2])
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if !ok {
		_ = w.NullBulk()
		return
	}
	_ = w.BulkString(formatFloat(score))
}

func (s *Server) cmdZRem(w *resp.Writer, args []string) {
	if len(args) < 3 {
		wrongArgs(w, args[0])
		return
	}
	n, err := s.store.ZRem(args[1], args[2:])
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	_ = w.Integer(n)
}

// formatFloat formats a score the way Redis does: trimmed of trailing
// zeros, no exponent, and "inf"/"-inf" for the infinities.
func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}
