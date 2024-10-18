// +build ignore

package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/format"
	"io/ioutil"
	"os"
	"runtime/debug"
	"text/template"

	"github.com/juju/juju/environs"
	_ "github.com/juju/juju/provider/all"
	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/version"
)

var file = flag.String("o", "", "`file` to write.")

func main() {
	flag.Parse()

	visibleAttributes := make(map[string]bool)
	for _, pname := range environs.RegisteredProviders() {
		p, err := environs.Provider(pname)
		if err != nil {
			panic(err)
		}
		for authtype, s := range p.CredentialSchemas() {
			for _, attr := range s {
				visibleAttributes[fmt.Sprintf("%s\x1e%s\x1e%s", pname, authtype, attr.Name)] = !attr.Hidden
			}
		}
	}

	p := params{
		JujuVersion: version.Current.String(),
		Attributes:  visibleAttributes,
	}

	bi, ok := debug.ReadBuildInfo()
	if ok {
		for _, d := range bi.Deps {
			if d.Path != "github.com/juju/juju" {
				continue
			}
			if d.Replace != nil {
				break
			}
			p.ModuleVersion = d.Version
			break
		}
	}

	b := new(bytes.Buffer)
	if err := tmpl.Execute(b, p); err != nil {
		panic(err)
	}

	formatted, err := format.Source(b.Bytes())
	if err != nil {
		panic(err)
	}

	if *file != "" {
		if err := ioutil.WriteFile(*file, formatted, 0664); err != nil {
			panic(err)
		}
	} else {
		os.Stdout.Write(formatted)
	}
}

type params struct {
	JujuVersion   string
	ModuleVersion string
	Attributes    map[string]bool
}

var tmpl = template.Must(template.New("").Parse(`
// GENERATED FILE - DO NOT EDIT
// 
// Generated from:
//   Juju Version:   {{.JujuVersion}}
//   Module Version: {{.ModuleVersion}}

package cloudcred

var attr = map[string]bool {
{{range $name, $value := .Attributes}}	{{printf "%q" $name}}: {{$value}},
{{end -}}
}
`[1:]))
