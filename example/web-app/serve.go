//go:build ignore

// Serves this directory on http://localhost:8080.
//
//	go run serve.go
//
// Any static server will do, with one requirement: .wasm has to arrive as
// application/wasm, or instantiateStreaming refuses it.  Go's http package
// knows that extension, and python3 -m http.server does not.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
)

func main() {
	addr := flag.String("addr", "localhost:8080", "the address to serve on")
	flag.Parse()

	fmt.Printf("http://%s\n", *addr)
	log.Fatal(http.ListenAndServe(*addr, http.FileServer(http.Dir("."))))
}
