// Copyright 2012 Jesse van den Kieboom. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package configure provides a very simple gnu configure/make style configure
// script generating a simple Makefile and go file containing all the configured
// variables.
package configure

import (
	"bytes"
	"fmt"
	"github.com/jessevdk/go-flags"
	"io"
	"os"
	"path"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"runtime"
)

// Options contains all the standard configure options to specify various
// directories. Use NewOptions to create an instance of this type with the
// common default values for each variable.
type Options struct {
	Prefix        string `long:"prefix" description:"install architecture-independent files in PREFIX"`
	ExecPrefix    string `long:"execprefix" description:"install architecture-dependent files in EPREFIX"`
	BinDir        string `long:"bindir" description:"user executables"`
	LibExecDir    string `long:"libexecdir" description:"program executables"`
	SysConfDir    string `long:"sysconfdir" description:"read-only single-machine data"`
	LibDir        string `long:"libdir" description:"program executables"`
	DataRootDir   string `long:"datarootdir" description:"read-only arch.-independent data root"`
	DataDir       string `long:"datadir" description:"read-only arc.-independent data"`
	ManDir        string `long:"mandir" description:"man documentation"`
}

// NewOptions creates a new Options with common default values.
func NewOptions() *Options {
	return &Options{
		Prefix:        "/usr/local",
		ExecPrefix:    "${prefix}",
		BinDir:        "${execprefix}/bin",
		LibExecDir:    "${execprefix}/libexec",
		LibDir:        "${execprefix}/lib",
		SysConfDir:    "${prefix}/etc",
		DataRootDir:   "${prefix}/share",
		DataDir:       "${datarootdir}",
		ManDir:        "${datarootdir}/man",
	}
}

// Package is the package name in which the GoConfig file will be written
var Package = "main"

// Makefile is the filename of the makefile that will be generated
var Makefile = "go.make"

// GoConfig is the filename of the go file that will be generated containing
// all the variable values.
var GoConfig = "appconfig"

// GoConfigVariable is the name of the variable inside the GoConfig file
// containing all the variable values.
var GoConfigVariable = "AppConfig"

// Target is the executable name to build. If left empty, the name is deduced
// from the directory (similar to what go does)
var Target = ""

// Version is the application version
var Version []int = []int{0, 1}

type expandStringPart struct {
	Value      string
	IsVariable bool
}

func (x *expandStringPart) expand(m map[string]*expandString) (string, []string) {
	if x.IsVariable {
		s, ok := m[x.Value]

		if !ok {
			return "", nil
		} else {
			ret := s.expand(m)
			rets := make([]string, len(s.dependencies), len(s.dependencies)+1)

			copy(rets, s.dependencies)

			return ret, append(rets, x.Value)
		}
	}

	return x.Value, nil
}

type expandString struct {
	Name  string
	Parts []expandStringPart

	dependencies []string
	value        string
	hasExpanded  bool
}

func (x *expandString) dependsOn(name string) bool {
	i := sort.SearchStrings(x.dependencies, name)

	return i < len(x.dependencies) && x.dependencies[i] == name
}

func (x *expandString) expand(m map[string]*expandString) string {
	if !x.hasExpanded {
		// Prevent infinite loop by circular dependencies
		x.hasExpanded = true
		buf := bytes.Buffer{}

		for _, v := range x.Parts {
			s, deps := v.expand(m)
			buf.WriteString(s)

			x.dependencies = append(x.dependencies, deps...)
		}

		sort.Strings(x.dependencies)
		x.value = buf.String()
	}

	return x.value
}

// Config represents the current configuration. See Configure for more
// information.
type Config struct {
	*flags.Parser

	values   map[string]interface{}
	expanded map[string]*expandString
}

func (x *Config) extract() map[string]interface{} {
	ret := make(map[string]interface{})

	for _, grp := range x.Parser.Groups {
		for longname, option := range grp.LongNames {
			ret[longname] = option.Value.Interface()
		}
	}

	return ret
}

func (x *Config) expand() map[string]*expandString {
	ret := make(map[string]*expandString)

	r, _ := regexp.Compile(`\$\{[^}]*\}`)

	for name, val := range x.values {
		es := expandString{
			Name: name,
		}

		// Find all variable references
		s := fmt.Sprintf("%v", val)

		matches := r.FindAllStringIndex(s, -1)

		for i, match := range matches {
			var prefix string

			if i == 0 {
				prefix = s[0:match[0]]
			} else {
				prefix = s[matches[i-1][1]:match[0]]
			}

			if len(prefix) != 0 {
				es.Parts = append(es.Parts, expandStringPart{Value: prefix, IsVariable: false})
			}

			varname := s[match[0]+2 : match[1]-1]
			es.Parts = append(es.Parts, expandStringPart{Value: varname, IsVariable: true})
		}

		if len(matches) == 0 {
			es.Parts = append(es.Parts, expandStringPart{Value: s, IsVariable: false})
		} else {
			last := matches[len(matches)-1]
			suffix := s[last[1]:]

			if len(suffix) != 0 {
				es.Parts = append(es.Parts, expandStringPart{Value: suffix, IsVariable: false})
			}
		}

		ret[name] = &es
	}

	for _, val := range ret {
		val.expand(ret)
	}

	return ret
}

// Configure runs the configure process with options as provided by the given
// data variable. If data is nil, the default options will be used
// (see NewOptions). Note that the data provided is simply passed to go-flags.
// For more information on flags parsing, see the documentation of go-flags.
// If GoConfig is not empty, then the go configuration will be written to the
// GoConfig file. Similarly, if Makefile is not empty, the Makefile will be
// written.
func Configure(data interface{}) (*Config, error) {
	if data == nil {
		data = NewOptions()
	}

	parser := flags.NewParser(data, flags.PrintErrors | flags.IgnoreUnknown)

	if _, err := parser.Parse(); err != nil {
		return nil, err
	}

	ret := &Config{
		Parser: parser,
	}

	ret.values = ret.extract()
	ret.expanded = ret.expand()

	if len(GoConfig) != 0 {
		filename := GoConfig

		if !strings.HasSuffix(filename, ".go") {
			filename += ".go"
		}

		f, err := os.Create(filename)

		if err != nil {
			return nil, err
		}

		ret.WriteGoConfig(f)
		f.Close()
	}

	if len(Makefile) != 0 {
		f, err := os.Create(Makefile)

		if err != nil {
			return nil, err
		}

		ret.WriteMakefile(f)
		f.Close()

		os.Chmod(Makefile, 0755)

		f, err = os.OpenFile(path.Join(path.Dir(Makefile), "Makefile"),
		                     os.O_CREATE | os.O_EXCL | os.O_WRONLY,
		                     0644)

		if err == nil {
			fmt.Fprintf(f, "include %s\n", path.Base(Makefile))
			f.Close()
		}
	}

	return ret, nil
}

// Expand expands the variable value indicated by name
func (x *Config) Expand(name string) string {
	return x.expanded[name].expand(x.expanded)
}

// WriteGoConfig writes the go configuration file containing all the variable
// values to the given writer. Note that it will write a package line if
// the Package variable is not empty. The GoConfigVariable name will
// be used as the variable name for the configuration.
func (x *Config) WriteGoConfig(writer io.Writer) {
	if len(Package) > 0 {
		fmt.Fprintf(writer, "package %v\n\n", Package)
	}

	fmt.Fprintf(writer, "var %s = struct {\n", GoConfigVariable)
	values := make([]string, 0)

	variables := make([]string, 0, len(x.values))
	optionmap := make(map[string]*flags.Option)

	// Write all options
	for _, grp := range x.Parser.Groups {
		for _, option := range grp.LongNames {
			name := option.Field.Name

			variables = append(variables, name)
			optionmap[name] = option
		}
	}

	sort.Strings(variables)

	for i, name := range variables {
		if i != 0 {
			io.WriteString(writer, "\n")
		}

		option := optionmap[name]
		val := option.Value.Interface()

		fmt.Fprintf(writer, "\t// %s\n", option.Description)
		fmt.Fprintf(writer, "\t%v %T\n", name, val)

		var value string

		if option.Value.Type().Kind() == reflect.String {
			value = fmt.Sprintf("%#v", x.Expand(option.LongName))
		} else {
			value = fmt.Sprintf("%#v", val)
		}

		values = append(values, value)
	}

	if len(variables) > 0 {
		io.WriteString(writer, "\n")
	}

	io.WriteString(writer, "\t// Application version\n")
	io.WriteString(writer, "\tVersion []int\n")
	fmt.Fprintln(writer, "}{")

	for _, v := range values {
		fmt.Fprintf(writer, "\t%v,\n", v)
	}

	for i, v := range Version {
		if i != 0 {
			io.WriteString(writer, ", ")
		} else {
			io.WriteString(writer, "\t[]int{")
		}

		fmt.Fprintf(writer, "%v", v)
	}

	fmt.Fprintln(writer, "},")
	fmt.Fprintln(writer, "}")
}

// WriteMakefile writes a Makefile for the given parser to the given writer.
// The Makefile contains the common build, clean, distclean, install and
// uninstall rules.
func (x *Config) WriteMakefile(writer io.Writer) {
	// Write a very basic makefile
	io.WriteString(writer, "#!/usr/bin/make -f\n\n")

	vars := make([]*expandString, 0, len(x.expanded))

	for name, v := range x.expanded {
		inserted := false

		// Insert into vars based on dependencies
		for i, vv := range vars {
			if vv.dependsOn(name) {
				tail := make([]*expandString, len(vars)-i)
				copy(tail, vars[i:])

				if i == 0 {
					vars = append([]*expandString{v}, vars...)
				} else {
					vars = append(append(vars[0:i], v), tail...)
				}

				inserted = true
				break
			}
		}

		if !inserted {
			vars = append(vars, v)
		}
	}

	io.WriteString(writer, "# Variables\n")

	for _, v := range vars {
		fmt.Fprintf(writer, "%s = ", v.Name)

		for _, part := range v.Parts {
			if part.IsVariable {
				fmt.Fprintf(writer, "$(%s)", part.Value)
			} else {
				fmt.Fprintf(writer, "%s", part.Value)
			}
		}

		io.WriteString(writer, "\n")
	}

	io.WriteString(writer, "version = ")

	for i, v := range Version {
		if i != 0 {
			io.WriteString(writer, ".")
		}

		fmt.Fprintf(writer, "%v", v)
	}

	io.WriteString(writer, "\n")
	fmt.Fprintf(writer, "major_version = %v\n", Version[0])

	if len(Version) > 1 {
		fmt.Fprintf(writer, "minor_version = %v\n", Version[0])
	}

	if len(Version) > 2 {
		fmt.Fprintf(writer, "micro_version = %v\n", Version[0])
	}

	io.WriteString(writer, "\n")

	target := Target

	if len(target) == 0 {
		pc := make([]uintptr, 3)
		n := runtime.Callers(1, pc)

		me, _ := runtime.FuncForPC(pc[0]).FileLine(pc[0])

		for i := 1; i < n; i++ {
			f := runtime.FuncForPC(pc[i])
			fname, _ := f.FileLine(pc[i])

			if fname != me {
				target = path.Base(path.Dir(fname))
				break
			}
		}
	}

	fmt.Fprintf(writer, "TARGET = %s\n", target)

	io.WriteString(writer, "\nSOURCES ?=")
	io.WriteString(writer, "\nSOURCES += $(wildcard *.go)")
	io.WriteString(writer, "\nSOURCES_UNIQUE = $(sort $(SOURCES))")

	io.WriteString(writer, "\n\n")

	io.WriteString(writer, "# Rules\n")
	io.WriteString(writer, "$(TARGET): $(SOURCES_UNIQUE)\n")
	io.WriteString(writer, "\tgo build -o $@\n\n")

	io.WriteString(writer, "clean:\n")
	io.WriteString(writer, "\trm -f $(TARGET)\n\n")

	io.WriteString(writer, "distclean: clean\n\n")

	io.WriteString(writer, "install: $(TARGET)\n")
	io.WriteString(writer, "\tmkdir -p $(DESTDIR)$(bindir) && cp $(TARGET) $(DESTDIR)$(bindir)/$(TARGET)\n\n")

	io.WriteString(writer, "uninstall:\n")
	io.WriteString(writer, "\trm -f $(DESTDIR)$(bindir)/$(TARGET)\n\n")

	io.WriteString(writer, ".PHONY: install uninstall distclean clean")
}
