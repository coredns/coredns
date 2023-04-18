//go:build ignore
// +build ignore

package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"log"
	"os"
	"strings"
	"text/template"
)

var packageHdr = `
// Code generated by coredns Atlas plugin; DO NOT EDIT.

package record

import (
	"encoding/json"
	"fmt"

	"github.com/miekg/dns"
	"github.com/coredns/coredns/plugin/atlas/ent"
)
`

var marshalFunc = template.Must(template.New("marshalFunc").Funcs(template.FuncMap{
	"toLower": func(input string) string {
		return strings.ToLower(input)
	},
}).Parse(`
{{range .}}
// Marshal {{.Name}} RR and return json string and error if any
func (rec {{.Name}}) Marshal() (s string, e error) { 
	var m []byte
	if m, e = json.Marshal(rec); e != nil {
		return
	}
	return string(m), nil
}

// New{{.Name}} creates a record.{{.Name}} from *dns.{{.Name}}
func New{{.Name}}(rec *dns.{{.Name}}) {{.Name}} {
	return {{.Name}}{
	{{range .Fields -}}
	{{.FieldName}}: rec.{{.FieldName}},
	{{end}}
	}
}
{{end}}

// From returns a dns.RR from ent.DnsRR
func From(rec *ent.DnsRR) (dns.RR, error) {
	if rec == nil {
		return nil, fmt.Errorf("unexpected DnsRR record")
	}

	header, err := GetRRHeaderFromDnsRR(rec)
	if err != nil {
		return nil, err
	}

	switch rec.Rrtype {
{{range . -}}
	{{ $n := .Name }}
    case dns.Type{{$n}}:
		var rec{{$n}} {{$n}}
		if err := json.Unmarshal([]byte(rec.Rrdata), &rec{{$n}}); err != nil {
			return nil, err
		}
		
		{{$n | toLower}} := dns.{{$n}}{
			{{if and (ne $n "KEY") (ne $n "CDNSKEY") -}}
			Hdr: *header,
			{{end -}}
			{{range .Fields -}}			
			{{.FieldName}}: rec{{$n}}.{{.FieldName}},
			{{end}}			
		}
		
		return &{{.Name | toLower}}, nil
{{end}}	
	default:
		return nil, fmt.Errorf("unknown dns.RR")
	}
}
`))

type Field struct {
	FieldName string
	Type      string
	IsArray   bool
}

type OutputType struct {
	Name   string
	Fields []Field
}

func main() {

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, "rr_types.go", nil, parser.ParseComments)
	if err != nil {
		log.Fatal(err)
	}

	outTypes := make([]OutputType, 0)

	for _, node := range node.Decls {
		switch node.(type) {

		case *ast.GenDecl:
			genDecl := node.(*ast.GenDecl)
			for _, spec := range genDecl.Specs {
				switch spec.(type) {
				case *ast.TypeSpec:
					typeSpec := spec.(*ast.TypeSpec)

					t := OutputType{Name: typeSpec.Name.Name}

					switch typeSpec.Type.(type) {
					case *ast.StructType:
						structType := typeSpec.Type.(*ast.StructType)
						for _, field := range structType.Fields.List {
							switch tp := field.Type.(type) {
							case *ast.Ident:
								fieldType := tp.Name
								for _, name := range field.Names {
									f := Field{FieldName: name.Name, Type: fieldType, IsArray: false}
									t.Fields = append(t.Fields, f)
								}

							case *ast.ArrayType:
								fieldType := fmt.Sprintf("[]%v", tp.Elt)
								for _, name := range field.Names {
									f := Field{FieldName: name.Name, Type: fieldType, IsArray: true}
									t.Fields = append(t.Fields, f)
								}

							case *ast.SelectorExpr:
								fieldType := fmt.Sprintf("%v.%v", tp.X, tp.Sel)
								for _, name := range field.Names {
									f := Field{FieldName: name.Name, Type: fieldType, IsArray: false}
									t.Fields = append(t.Fields, f)
								}

							default:
								fmt.Printf("default: %+v => %T\n", tp, tp)
							}
						}
					}
					outTypes = append(outTypes, t)
				}

			}
		}
	}

	b := &bytes.Buffer{}
	b.WriteString(packageHdr)

	if err := marshalFunc.Execute(b, outTypes); err != nil {
		log.Panic(err)
	}

	file, _ := os.Create("rr_types_generated.go")
	defer file.Close()

	formatted, err := format.Source(b.Bytes())
	if err != nil {
		log.Panic(err)
	}

	if _, err := file.Write(formatted); err != nil {
		log.Panic(err)
	}
}
