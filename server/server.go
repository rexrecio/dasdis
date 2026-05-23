// Package server hosts the TCP listener and dispatches RESP commands.
package server

import (
	"errors"
	"io"
	"log"
	"net"
	"strings"

	"github.com/rexrecio/dasdis/resp"
	"github.com/rexrecio/dasdis/store"
)

type Server struct {
	store *store.Store
}

func New() *Server {
	return &Server{store: store.New()}
}

func (s *Server) ListenAndServe(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	log.Printf("dasdis listening on %s", addr)
	for {
		c, err := ln.Accept()
		if err != nil {
			return err
		}
		go s.handle(c)
	}
}

func (s *Server) handle(c net.Conn) {
	defer c.Close()
	r := resp.NewReader(c)
	w := resp.NewWriter(c)
	for {
		args, err := r.ReadCommand()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				log.Printf("read: %v", err)
			}
			return
		}
		if len(args) == 0 {
			continue
		}
		quit := s.dispatch(w, args)
		if err := w.Flush(); err != nil {
			return
		}
		if quit {
			return
		}
	}
}

// dispatch executes one command and returns true when the connection should close.
func (s *Server) dispatch(w *resp.Writer, args []string) bool {
	cmd := strings.ToUpper(args[0])
	switch cmd {
	case "QUIT":
		_ = w.SimpleString("OK")
		return true
	case "PING":
		s.cmdPing(w, args)
	case "ECHO":
		s.cmdEcho(w, args)
	case "COMMAND":
		_ = w.ArrayHeader(0)
	case "SELECT", "CLIENT":
		_ = w.SimpleString("OK")
	case "HELLO":
		s.cmdHello(w, args)
	case "DEL":
		s.cmdDel(w, args)
	case "EXISTS":
		s.cmdExists(w, args)
	case "GET":
		s.cmdGet(w, args)
	case "SET":
		s.cmdSet(w, args)
	case "INCR":
		s.cmdIncrBy(w, args[0], args[1:], 1)
	case "DECR":
		s.cmdIncrBy(w, args[0], args[1:], -1)
	case "LPUSH":
		s.cmdPush(w, args, true)
	case "RPUSH":
		s.cmdPush(w, args, false)
	case "LPOP":
		s.cmdPop(w, args, true)
	case "RPOP":
		s.cmdPop(w, args, false)
	case "LLEN":
		s.cmdLLen(w, args)
	case "LRANGE":
		s.cmdLRange(w, args)
	case "ZADD":
		s.cmdZAdd(w, args)
	case "ZRANGE":
		s.cmdZRange(w, args)
	case "ZSCORE":
		s.cmdZScore(w, args)
	case "ZREM":
		s.cmdZRem(w, args)
	default:
		_ = w.Error("ERR unknown command '" + args[0] + "'")
	}
	return false
}

func wrongArgs(w *resp.Writer, name string) {
	_ = w.Error("ERR wrong number of arguments for '" + strings.ToLower(name) + "' command")
}

func writeStoreErr(w *resp.Writer, err error) {
	_ = w.Error(err.Error())
}
