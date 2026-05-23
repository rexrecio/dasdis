package server

import (
	"bufio"
	"net"
	"strings"
	"testing"
	"time"
)

func startServer(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	srv := New()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go srv.handle(c)
		}
	}()
	return ln.Addr().String()
}

type client struct {
	c net.Conn
	r *bufio.Reader
}

func dial(t *testing.T, addr string) *client {
	t.Helper()
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	return &client{c: c, r: bufio.NewReader(c)}
}

func (cl *client) send(t *testing.T, args ...string) {
	t.Helper()
	var sb strings.Builder
	sb.WriteByte('*')
	sb.WriteString(itoa(len(args)))
	sb.WriteString("\r\n")
	for _, a := range args {
		sb.WriteByte('$')
		sb.WriteString(itoa(len(a)))
		sb.WriteString("\r\n")
		sb.WriteString(a)
		sb.WriteString("\r\n")
	}
	if _, err := cl.c.Write([]byte(sb.String())); err != nil {
		t.Fatal(err)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func (cl *client) readLine(t *testing.T) string {
	t.Helper()
	line, err := cl.r.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(line, "\r\n") {
		t.Fatalf("bad line %q", line)
	}
	return line[:len(line)-2]
}

func (cl *client) readBulk(t *testing.T) (string, bool) {
	t.Helper()
	line := cl.readLine(t)
	if line == "$-1" {
		return "", false
	}
	if !strings.HasPrefix(line, "$") {
		t.Fatalf("expected bulk, got %q", line)
	}
	// read the payload + CRLF
	data := cl.readLine(t)
	return data, true
}

func TestStringsAndConnection(t *testing.T) {
	addr := startServer(t)
	c := dial(t, addr)

	c.send(t, "PING")
	if got := c.readLine(t); got != "+PONG" {
		t.Fatalf("PING: %q", got)
	}

	c.send(t, "ECHO", "hi")
	if v, ok := c.readBulk(t); !ok || v != "hi" {
		t.Fatalf("ECHO: %q ok=%v", v, ok)
	}

	c.send(t, "SET", "k", "v")
	if got := c.readLine(t); got != "+OK" {
		t.Fatalf("SET: %q", got)
	}

	c.send(t, "GET", "k")
	if v, ok := c.readBulk(t); !ok || v != "v" {
		t.Fatalf("GET: %q ok=%v", v, ok)
	}

	c.send(t, "GET", "missing")
	if _, ok := c.readBulk(t); ok {
		t.Fatalf("GET missing should be null")
	}

	c.send(t, "INCR", "counter")
	if got := c.readLine(t); got != ":1" {
		t.Fatalf("INCR: %q", got)
	}
	c.send(t, "INCR", "counter")
	if got := c.readLine(t); got != ":2" {
		t.Fatalf("INCR: %q", got)
	}
	c.send(t, "DECR", "counter")
	if got := c.readLine(t); got != ":1" {
		t.Fatalf("DECR: %q", got)
	}

	c.send(t, "EXISTS", "k", "missing", "counter")
	if got := c.readLine(t); got != ":2" {
		t.Fatalf("EXISTS: %q", got)
	}

	c.send(t, "DEL", "k", "counter", "missing")
	if got := c.readLine(t); got != ":2" {
		t.Fatalf("DEL: %q", got)
	}
}

func TestLists(t *testing.T) {
	addr := startServer(t)
	c := dial(t, addr)

	c.send(t, "RPUSH", "mylist", "a", "b", "c")
	if got := c.readLine(t); got != ":3" {
		t.Fatalf("RPUSH: %q", got)
	}
	c.send(t, "LPUSH", "mylist", "z")
	if got := c.readLine(t); got != ":4" {
		t.Fatalf("LPUSH: %q", got)
	}
	c.send(t, "LLEN", "mylist")
	if got := c.readLine(t); got != ":4" {
		t.Fatalf("LLEN: %q", got)
	}

	c.send(t, "LRANGE", "mylist", "0", "-1")
	header := c.readLine(t)
	if header != "*4" {
		t.Fatalf("LRANGE header: %q", header)
	}
	want := []string{"z", "a", "b", "c"}
	for _, w := range want {
		v, ok := c.readBulk(t)
		if !ok || v != w {
			t.Fatalf("LRANGE want %q got %q ok=%v", w, v, ok)
		}
	}

	c.send(t, "LPOP", "mylist")
	if v, ok := c.readBulk(t); !ok || v != "z" {
		t.Fatalf("LPOP: %q ok=%v", v, ok)
	}
	c.send(t, "RPOP", "mylist")
	if v, ok := c.readBulk(t); !ok || v != "c" {
		t.Fatalf("RPOP: %q ok=%v", v, ok)
	}

	// drain
	c.send(t, "LPOP", "mylist")
	_, _ = c.readBulk(t)
	c.send(t, "LPOP", "mylist")
	_, _ = c.readBulk(t)
	c.send(t, "EXISTS", "mylist")
	if got := c.readLine(t); got != ":0" {
		t.Fatalf("EXISTS after drain: %q", got)
	}
}

func TestZSet(t *testing.T) {
	addr := startServer(t)
	c := dial(t, addr)

	c.send(t, "ZADD", "z", "1", "a", "2", "b", "3", "c")
	if got := c.readLine(t); got != ":3" {
		t.Fatalf("ZADD: %q", got)
	}

	// re-add with same score: no new members
	c.send(t, "ZADD", "z", "2", "b")
	if got := c.readLine(t); got != ":0" {
		t.Fatalf("ZADD dup: %q", got)
	}

	// update score
	c.send(t, "ZADD", "z", "10", "a")
	if got := c.readLine(t); got != ":0" {
		t.Fatalf("ZADD update: %q", got)
	}

	// negative score, should sort before everything
	c.send(t, "ZADD", "z", "-5", "neg")
	if got := c.readLine(t); got != ":1" {
		t.Fatalf("ZADD neg: %q", got)
	}

	c.send(t, "ZRANGE", "z", "0", "-1", "WITHSCORES")
	if got := c.readLine(t); got != "*8" {
		t.Fatalf("ZRANGE header: %q", got)
	}
	want := []struct{ m, s string }{
		{"neg", "-5"}, {"b", "2"}, {"c", "3"}, {"a", "10"},
	}
	for _, p := range want {
		m, _ := c.readBulk(t)
		s, _ := c.readBulk(t)
		if m != p.m || s != p.s {
			t.Fatalf("ZRANGE got %q,%q want %q,%q", m, s, p.m, p.s)
		}
	}

	c.send(t, "ZSCORE", "z", "a")
	if v, ok := c.readBulk(t); !ok || v != "10" {
		t.Fatalf("ZSCORE a: %q ok=%v", v, ok)
	}
	c.send(t, "ZSCORE", "z", "missing")
	if _, ok := c.readBulk(t); ok {
		t.Fatalf("ZSCORE missing should be null")
	}

	c.send(t, "ZREM", "z", "neg", "missing")
	if got := c.readLine(t); got != ":1" {
		t.Fatalf("ZREM: %q", got)
	}
}

func TestWrongType(t *testing.T) {
	addr := startServer(t)
	c := dial(t, addr)

	c.send(t, "SET", "k", "v")
	_ = c.readLine(t)
	c.send(t, "LPUSH", "k", "x")
	got := c.readLine(t)
	if !strings.HasPrefix(got, "-WRONGTYPE") {
		t.Fatalf("expected WRONGTYPE error, got %q", got)
	}
}
