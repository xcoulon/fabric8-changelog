package cmd

import (
	"log"
	"text/template"
)

func newTextTemplate(name, src string, funcs ...template.FuncMap) template.Template {
	t := template.New(name)
	for _, f := range funcs {
		t.Funcs(f)
	}
	t, err := t.Parse(src)
	if err != nil {
		log.Fatalf("failed to initialize '%s' template: %s", name, err.Error())
	}
	return *t
}
