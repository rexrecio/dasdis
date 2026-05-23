// Package resp implements a minimal RESP2 reader and writer.
package resp

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
)

var ErrInvalidProtocol = errors.New("resp: invalid protocol")

// Reader reads RESP2 frames from a connection.
type Reader struct {
	r *bufio.Reader
}

func NewReader(r io.Reader) *Reader {
	return &Reader{r: bufio.NewReader(r)}
}

// ReadCommand reads one inbound command as a slice of arguments.
// Redis clients always send commands as an Array of Bulk Strings, but the
// inline form (a CRLF-terminated text line) is also accepted for convenience
// when using telnet.
func (r *Reader) ReadCommand() ([]string, error) {
	prefix, err := r.r.ReadByte()
	if err != nil {
		return nil, err
	}
	if prefix != '*' {
		if err := r.r.UnreadByte(); err != nil {
			return nil, err
		}
		return r.readInline()
	}
	n, err := r.readInt()
	if err != nil {
		return nil, err
	}
	if n < 0 {
		return nil, nil
	}
	args := make([]string, 0, n)
	for i := int64(0); i < n; i++ {
		s, err := r.readBulkString()
		if err != nil {
			return nil, err
		}
		args = append(args, s)
	}
	return args, nil
}

func (r *Reader) readInline() ([]string, error) {
	line, err := r.readLine()
	if err != nil {
		return nil, err
	}
	if len(line) == 0 {
		return nil, nil
	}
	return splitInline(line), nil
}

func splitInline(line string) []string {
	var out []string
	start := -1
	for i := 0; i < len(line); i++ {
		c := line[i]
		if c == ' ' || c == '\t' {
			if start >= 0 {
				out = append(out, line[start:i])
				start = -1
			}
			continue
		}
		if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		out = append(out, line[start:])
	}
	return out
}

func (r *Reader) readBulkString() (string, error) {
	prefix, err := r.r.ReadByte()
	if err != nil {
		return "", err
	}
	if prefix != '$' {
		return "", fmt.Errorf("%w: expected $ got %q", ErrInvalidProtocol, prefix)
	}
	n, err := r.readInt()
	if err != nil {
		return "", err
	}
	if n < 0 {
		return "", nil
	}
	buf := make([]byte, n+2)
	if _, err := io.ReadFull(r.r, buf); err != nil {
		return "", err
	}
	if buf[n] != '\r' || buf[n+1] != '\n' {
		return "", ErrInvalidProtocol
	}
	return string(buf[:n]), nil
}

func (r *Reader) readInt() (int64, error) {
	line, err := r.readLine()
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(line, 10, 64)
}

func (r *Reader) readLine() (string, error) {
	line, err := r.r.ReadString('\n')
	if err != nil {
		return "", err
	}
	if len(line) < 2 || line[len(line)-2] != '\r' {
		return "", ErrInvalidProtocol
	}
	return line[:len(line)-2], nil
}

// Writer writes RESP2 frames to a connection.
type Writer struct {
	w *bufio.Writer
}

func NewWriter(w io.Writer) *Writer {
	return &Writer{w: bufio.NewWriter(w)}
}

func (w *Writer) Flush() error { return w.w.Flush() }

func (w *Writer) SimpleString(s string) error {
	if _, err := w.w.WriteString("+" + s + "\r\n"); err != nil {
		return err
	}
	return nil
}

func (w *Writer) Error(s string) error {
	_, err := w.w.WriteString("-" + s + "\r\n")
	return err
}

func (w *Writer) Integer(n int64) error {
	_, err := w.w.WriteString(":" + strconv.FormatInt(n, 10) + "\r\n")
	return err
}

func (w *Writer) BulkString(s string) error {
	if _, err := w.w.WriteString("$" + strconv.Itoa(len(s)) + "\r\n"); err != nil {
		return err
	}
	if _, err := w.w.WriteString(s); err != nil {
		return err
	}
	_, err := w.w.WriteString("\r\n")
	return err
}

func (w *Writer) NullBulk() error {
	_, err := w.w.WriteString("$-1\r\n")
	return err
}

func (w *Writer) NullArray() error {
	_, err := w.w.WriteString("*-1\r\n")
	return err
}

func (w *Writer) ArrayHeader(n int) error {
	_, err := w.w.WriteString("*" + strconv.Itoa(n) + "\r\n")
	return err
}

// StringArray writes an array of bulk strings.
func (w *Writer) StringArray(items []string) error {
	if err := w.ArrayHeader(len(items)); err != nil {
		return err
	}
	for _, s := range items {
		if err := w.BulkString(s); err != nil {
			return err
		}
	}
	return nil
}
