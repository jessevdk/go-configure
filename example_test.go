// Example of use of the configure package.
package configure_test

func Example() {
	// Example configuration. Note that normally the default values in
	// the configure package are good enough.
	configure.Package = "main"
	configure.Makefile = "Makefile"
	configure.GoConfig = "AppConfig"
	configure.GoConfigVariable = "AppConfig"
	configure.Target = "example"

	// Generate Makefile and AppConfig.go using the default
	// configure options
	configure.Configure(nil)
}
