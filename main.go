package main

import (
	"flag"
	"log"

	"github.com/rexrecio/dasdis/server"
)

func main() {
	addr := flag.String("addr", ":6380", "address to listen on")
	flag.Parse()

	if err := server.New().ListenAndServe(*addr); err != nil {
		log.Fatal(err)
	}
}
