package sketch

import (
	"fmt"
	"io/fs"
	"strings"
	"text/template"

	"github.com/lestrrat-go/multifs"
	"github.com/lestrrat-go/sketch/schema"
)

type Template struct {
	srcs map[string]fs.FS
}

func (tmpl *Template) AddFS(prefix string, src fs.FS) {
	if tmpl.srcs == nil {
		tmpl.srcs = make(map[string]fs.FS)
	}
	tmpl.srcs[prefix] = src
}

func (tmpl *Template) Build() (*template.Template, error) {
	var mfs multifs.FS
	for prefix, sub := range tmpl.srcs {
		if err := mfs.Mount(prefix, sub); err != nil {
			return nil, fmt.Errorf(`failed to mount file system on %q: %w`, prefix, err)
		}
	}

	var files []string
	fs.WalkDir(&mfs, ".", func(name string, d fs.DirEntry, err error) error {
		name = "/" + name
		if err != nil {
			return nil
		}

		if d.IsDir() {
			return nil
		}

		if strings.HasSuffix(name, ".tmpl") {
			files = append(files, name)
		}
		return nil
	})

	var tt *template.Template
	tt = template.New("").Funcs(tmpl.makeFuncs(&tt))
	_, err := tt.ParseFS(&mfs, files...)
	if err != nil {
		return nil, fmt.Errorf(`failed to parse templates: %w`, err)
	}

	return tt, nil
}

func (tmpl *Template) makeFuncs(tt **template.Template) template.FuncMap {
	return template.FuncMap{
		"comment":     tmpl.comment(tt),
		"hasTemplate": tmpl.hasTemplate(tt),
		"runTemplate": tmpl.runTemplate(tt),
		"fieldByName": tmpl.fieldByName(tt),
	}
}

func (tmpl *Template) comment(tt **template.Template) func(string, interface{}) string {
	return func(src string, arg interface{}) string {
		if src == "" {
			return ""
		}

		tmpl, err := template.New("comment").Parse(src)
		if err != nil {
			panic(err)
		}

		var sb strings.Builder
		sb.WriteString(`// `)
		if err := tmpl.Execute(&sb, arg); err != nil {
			panic(err)
		}
		return sb.String()
	}
}

func (tmpl *Template) hasTemplate(tt **template.Template) func(string) bool {
	return func(name string) bool {
		return (*tt).Lookup(name) != nil
	}
}

func (tmpl *Template) runTemplate(tt **template.Template) func(string, interface{}) (string, error) {
	return func(name string, vars interface{}) (string, error) {
		var sb strings.Builder
		if err := (*tt).ExecuteTemplate(&sb, name, vars); err != nil {
			return "", err
		}
		return sb.String(), nil
	}
}

func (tmpl *Template) fieldByName(**template.Template) func(schema.Interface, string) *schema.FieldSpec {
	return func(s schema.Interface, name string) *schema.FieldSpec {
		for _, f := range s.Fields() {
			if f.GetName() == name {
				return f
			}
		}
		return nil
	}
}
