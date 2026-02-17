package claude

import (
	"bytes"
	"fmt"
	"io/fs"
	"text/template"
)

// PromptTemplate wraps a parsed Go template for prompt rendering.
type PromptTemplate struct {
	tmpl *template.Template
}

// LoadPromptTemplate loads and parses a template file from an fs.FS.
// The funcMap parameter is optional and may be nil.
func LoadPromptTemplate(fsys fs.FS, path string, funcMap template.FuncMap) (*PromptTemplate, error) {
	data, err := fs.ReadFile(fsys, path)
	if err != nil {
		return nil, fmt.Errorf("reading template %s: %w", path, err)
	}

	tmpl := template.New(path)
	if funcMap != nil {
		tmpl = tmpl.Funcs(funcMap)
	}

	tmpl, err = tmpl.Parse(string(data))
	if err != nil {
		return nil, fmt.Errorf("parsing template %s: %w", path, err)
	}

	return &PromptTemplate{tmpl: tmpl}, nil
}

// Render executes the template with the given data and returns the result.
func (pt *PromptTemplate) Render(data any) (string, error) {
	var buf bytes.Buffer
	if err := pt.tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}
	return buf.String(), nil
}
