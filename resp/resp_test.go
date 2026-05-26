package resp

import (
	"bytes"
	"strings"
	"testing"
)

func TestReadCommand_Array(t *testing.T) {
	r := NewReader(strings.NewReader("*2\r\n$3\r\nGET\r\n$3\r\nfoo\r\n"))
	got, err := r.ReadCommand()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "GET" || got[1] != "foo" {
		t.Fatalf("unexpected %v", got)
	}
}

func TestReadCommand_Inline(t *testing.T) {
	r := NewReader(strings.NewReader("PING\r\n"))
	got, err := r.ReadCommand()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "PING" {
		t.Fatalf("unexpected %v", got)
	}
}

func TestReadCommand_InlineMultiWord(t *testing.T) {
	r := NewReader(strings.NewReader("SET foo bar\r\n"))
	got, err := r.ReadCommand()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"SET", "foo", "bar"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestReadCommand_NullArray(t *testing.T) {
	r := NewReader(strings.NewReader("*-1\r\n"))
	got, err := r.ReadCommand()
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestReadCommand_InvalidBulkPrefix(t *testing.T) {
	r := NewReader(strings.NewReader("*1\r\n:3\r\n"))
	_, err := r.ReadCommand()
	if err == nil {
		t.Fatal("expected error for invalid bulk string prefix")
	}
}

func TestSplitInline(t *testing.T) {
	cases := []struct {
		in  string
		out []string
	}{
		{"SET foo bar", []string{"SET", "foo", "bar"}},
		{"  SET  foo  ", []string{"SET", "foo"}},
		{"PING", []string{"PING"}},
		{"", nil},
	}
	for _, c := range cases {
		got := splitInline(c.in)
		if len(got) != len(c.out) {
			t.Fatalf("splitInline(%q) = %v, want %v", c.in, got, c.out)
		}
		for i := range got {
			if got[i] != c.out[i] {
				t.Fatalf("splitInline(%q)[%d] = %q, want %q", c.in, i, got[i], c.out[i])
			}
		}
	}
}

func TestWriter_SimpleString(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	_ = w.SimpleString("OK")
	_ = w.Flush()
	if got := buf.String(); got != "+OK\r\n" {
		t.Fatalf("got %q", got)
	}
}

func TestWriter_Error(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	_ = w.Error("ERR bad")
	_ = w.Flush()
	if got := buf.String(); got != "-ERR bad\r\n" {
		t.Fatalf("got %q", got)
	}
}

func TestWriter_Integer(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	_ = w.Integer(42)
	_ = w.Flush()
	if got := buf.String(); got != ":42\r\n" {
		t.Fatalf("got %q", got)
	}
}

func TestWriter_BulkString(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	_ = w.BulkString("hello")
	_ = w.Flush()
	if got := buf.String(); got != "$5\r\nhello\r\n" {
		t.Fatalf("got %q", got)
	}
}

func TestWriter_NullBulk(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	_ = w.NullBulk()
	_ = w.Flush()
	if got := buf.String(); got != "$-1\r\n" {
		t.Fatalf("got %q", got)
	}
}

func TestWriter_NullArray(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	_ = w.NullArray()
	_ = w.Flush()
	if got := buf.String(); got != "*-1\r\n" {
		t.Fatalf("got %q", got)
	}
}

func TestWriter_StringArray(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	_ = w.StringArray([]string{"foo", "bar"})
	_ = w.Flush()
	want := "*2\r\n$3\r\nfoo\r\n$3\r\nbar\r\n"
	if got := buf.String(); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestWriterReader_RoundTrip(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	_ = w.StringArray([]string{"SET", "mykey", "myval"})
	_ = w.Flush()

	r := NewReader(&buf)
	args, err := r.ReadCommand()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"SET", "mykey", "myval"}
	if len(args) != len(want) {
		t.Fatalf("got %v, want %v", args, want)
	}
	for i := range args {
		if args[i] != want[i] {
			t.Fatalf("args[%d] = %q, want %q", i, args[i], want[i])
		}
	}
}
