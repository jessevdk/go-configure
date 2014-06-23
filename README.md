go-configure: a go library for gnu style configure scripts
==========================================================

This library provides a simple way to create a configure executable for use
with systems that handle gnu configure/make style projects.

Example (configure.go):
-----------------------
	import (
		"github.com/jessevdk/go-configure"
	)

	func main() {
		configure.Configure(nil)
	}

This is the most basic example to use the library. Run the configure "script"
with `go run configure.go` to output a Makefile and a appconfig.go file
containing all the configured variables in an easy to access go variable.

More information can be found in the documentation: <http://godoc.org/github.com/jessevdk/go-configure>
