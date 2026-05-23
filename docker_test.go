package main_test

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func buildContainer(ctx context.Context, t *testing.T) testcontainers.Container {
	t.Helper()
	req := testcontainers.ContainerRequest{
		FromDockerfile: testcontainers.FromDockerfile{
			Context:    ".",
			Dockerfile: "Dockerfile",
		},
		ExposedPorts: []string{"6380/tcp"},
		WaitingFor:   wait.ForListeningPort("6380/tcp").WithStartupTimeout(2 * time.Minute),
	}
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("start container: %v", err)
	}
	t.Cleanup(func() { _ = c.Terminate(ctx) })
	return c
}

func containerAddr(ctx context.Context, t *testing.T, c testcontainers.Container) string {
	t.Helper()
	host, err := c.Host(ctx)
	if err != nil {
		t.Fatalf("container host: %v", err)
	}
	port, err := c.MappedPort(ctx, "6380")
	if err != nil {
		t.Fatalf("container port: %v", err)
	}
	return fmt.Sprintf("%s:%s", host, port.Port())
}

type conn struct {
	c net.Conn
	r *bufio.Reader
}

func dialAddr(t *testing.T, addr string) *conn {
	t.Helper()
	c, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	_ = c.SetDeadline(time.Now().Add(10 * time.Second))
	t.Cleanup(func() { c.Close() })
	return &conn{c: c, r: bufio.NewReader(c)}
}

func (cn *conn) send(t *testing.T, args ...string) {
	t.Helper()
	var b strings.Builder
	fmt.Fprintf(&b, "*%d\r\n", len(args))
	for _, a := range args {
		fmt.Fprintf(&b, "$%d\r\n%s\r\n", len(a), a)
	}
	if _, err := cn.c.Write([]byte(b.String())); err != nil {
		t.Fatalf("send: %v", err)
	}
}

func (cn *conn) readLine(t *testing.T) string {
	t.Helper()
	line, err := cn.r.ReadString('\n')
	if err != nil {
		t.Fatalf("readLine: %v", err)
	}
	return strings.TrimSuffix(line, "\r\n")
}

func (cn *conn) readBulk(t *testing.T) (string, bool) {
	t.Helper()
	line := cn.readLine(t)
	if line == "$-1" {
		return "", false
	}
	if !strings.HasPrefix(line, "$") {
		t.Fatalf("expected bulk string, got %q", line)
	}
	return cn.readLine(t), true
}

func TestDockerImage(t *testing.T) {
	ctx := context.Background()
	c := buildContainer(ctx, t)
	addr := containerAddr(ctx, t, c)
	cl := dialAddr(t, addr)

	t.Run("PING", func(t *testing.T) {
		cl.send(t, "PING")
		if got := cl.readLine(t); got != "+PONG" {
			t.Fatalf("want +PONG, got %q", got)
		}
	})

	t.Run("SET_GET", func(t *testing.T) {
		cl.send(t, "SET", "foo", "bar")
		if got := cl.readLine(t); got != "+OK" {
			t.Fatalf("SET: want +OK, got %q", got)
		}
		cl.send(t, "GET", "foo")
		if v, ok := cl.readBulk(t); !ok || v != "bar" {
			t.Fatalf("GET: want bar ok=true, got %q ok=%v", v, ok)
		}
	})

	t.Run("GET_missing", func(t *testing.T) {
		cl.send(t, "GET", "no-such-key")
		if _, ok := cl.readBulk(t); ok {
			t.Fatal("GET missing key should return null bulk")
		}
	})

	t.Run("INCR_DECR", func(t *testing.T) {
		cl.send(t, "INCR", "n")
		if got := cl.readLine(t); got != ":1" {
			t.Fatalf("INCR: want :1, got %q", got)
		}
		cl.send(t, "INCR", "n")
		if got := cl.readLine(t); got != ":2" {
			t.Fatalf("INCR: want :2, got %q", got)
		}
		cl.send(t, "DECR", "n")
		if got := cl.readLine(t); got != ":1" {
			t.Fatalf("DECR: want :1, got %q", got)
		}
	})

	t.Run("DEL_EXISTS", func(t *testing.T) {
		cl.send(t, "SET", "x", "1")
		_ = cl.readLine(t)
		cl.send(t, "EXISTS", "x", "foo")
		if got := cl.readLine(t); got != ":2" {
			t.Fatalf("EXISTS: want :2, got %q", got)
		}
		cl.send(t, "DEL", "x", "foo", "n")
		if got := cl.readLine(t); got != ":3" {
			t.Fatalf("DEL: want :3, got %q", got)
		}
	})
}
