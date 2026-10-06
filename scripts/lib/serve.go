//go:build ignore

// Serves a folder over HTTP, for scripts/test-repo.sh: the site as a web
// host would, to a container that adds its repository.
//
//	go run scripts/lib/serve.go DIR PORT
package main

import (
	"log"
	"net/http"
	"os"
)

func main() {
	log.Fatal(http.ListenAndServe(":"+os.Args[2], http.FileServer(http.Dir(os.Args[1]))))
}
