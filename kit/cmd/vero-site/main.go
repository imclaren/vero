// Command vero-site serves a vero app's release site - the folder
// vero-repo build writes - on its own: the page with the download links,
// the repositories people's systems update from, /download?for=SYSTEM, and
// latest.json. It's for an app without a web server of its own; an app with
// one serves the same with kit/site's Handler.
//
//	vero-site -dir dist/site -addr :8080
//	vero-site -dir /srv/myapp -domain downloads.example.com   # HTTPS on :443
//
// With -domain it gets a certificate from Let's Encrypt for that name, keeps
// it in -certs, and redirects plain HTTP to HTTPS; the name has to point at
// this server, and ports 80 and 443 have to reach it. To publish a release,
// copy the new site over the old, for example with rsync, and vero-site
// serves it straight away.
package main

import (
	"crypto/tls"
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/imclaren/vero/kit/site"
	"golang.org/x/crypto/acme/autocert"
)

func main() {
	dir := flag.String("dir", "dist/site", "the folder vero-repo build wrote")
	addr := flag.String("addr", ":8080", "where to listen without -domain")
	domain := flag.String("domain", "", "serve HTTPS for this name, with a certificate from Let's Encrypt")
	certs := flag.String("certs", "", "where to keep certificates (default: a folder beside -dir)")
	flag.Parse()

	if _, err := os.Stat(filepath.Join(*dir, "index.html")); err != nil {
		log.Printf("warning: %s has no index.html; run vero-repo build without --no-page for a front page", *dir)
	}
	handler := logged(site.Handler(site.Options{Dir: *dir}))

	if *domain == "" {
		log.Printf("serving %s on %s", *dir, *addr)
		log.Fatal(server(*addr, handler).ListenAndServe())
	}
	if *certs == "" {
		*certs = filepath.Join(filepath.Dir(filepath.Clean(*dir)), "vero-site-certs")
	}
	m := &autocert.Manager{Prompt: autocert.AcceptTOS, HostPolicy: autocert.HostWhitelist(*domain), Cache: autocert.DirCache(*certs)}
	go func() { log.Fatal(server(":80", m.HTTPHandler(nil)).ListenAndServe()) }()
	s := server(":443", handler)
	s.TLSConfig = &tls.Config{GetCertificate: m.GetCertificate, MinVersion: tls.VersionTLS12}
	log.Printf("serving %s as https://%s", *dir, *domain)
	log.Fatal(s.ListenAndServeTLS("", ""))
}

func server(addr string, h http.Handler) *http.Server {
	return &http.Server{Addr: addr, Handler: h, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 2 * time.Minute}
}

// logged notes each request: its path, what was answered, and how long it took.
func logged(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &status{ResponseWriter: w, code: http.StatusOK}
		h.ServeHTTP(rec, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.RequestURI(), rec.code, time.Since(start).Round(time.Millisecond))
	})
}

type status struct {
	http.ResponseWriter
	code int
}

func (s *status) WriteHeader(code int) { s.code = code; s.ResponseWriter.WriteHeader(code) }
