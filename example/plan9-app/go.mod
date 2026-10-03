module github.com/imclaren/vero/example/plan9-app

go 1.24

// A module of its own, so that vero itself keeps having no dependencies: the
// one this example needs is a Go port of libdraw, and nothing else wants it.
require (
	9fans.net/go v0.0.8-0.20260825183529-7dfa0e8c5041
	github.com/imclaren/vero v0.0.0
)

replace github.com/imclaren/vero => ../..

replace 9fans.net/go => github.com/imclaren/9fans-go v0.0.8-0.20261003011416-5961d14f794b
