package claude

import (
	"strings"
	"testing"
	"testing/fstest"
	"text/template"
)

func TestLoadPromptTemplate(t *testing.T) {
	fsys := fstest.MapFS{
		"test.md": &fstest.MapFile{
			Data: []byte("Hello {{.Name}}, you have {{.Count}} items."),
		},
	}

	pt, err := LoadPromptTemplate(fsys, "test.md", nil)
	if err != nil {
		t.Fatalf("LoadPromptTemplate failed: %v", err)
	}

	result, err := pt.Render(struct {
		Name  string
		Count int
	}{Name: "Alice", Count: 3})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	expected := "Hello Alice, you have 3 items."
	if result != expected {
		t.Errorf("got %q, want %q", result, expected)
	}
}

func TestLoadPromptTemplateWithFuncMap(t *testing.T) {
	fsys := fstest.MapFS{
		"test.md": &fstest.MapFile{
			Data: []byte("Items: {{join .Items \", \"}}"),
		},
	}

	funcMap := template.FuncMap{
		"join": func(items []string, sep string) string {
			return strings.Join(items, sep)
		},
	}

	pt, err := LoadPromptTemplate(fsys, "test.md", funcMap)
	if err != nil {
		t.Fatalf("LoadPromptTemplate failed: %v", err)
	}

	result, err := pt.Render(struct {
		Items []string
	}{Items: []string{"a", "b", "c"}})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	expected := "Items: a, b, c"
	if result != expected {
		t.Errorf("got %q, want %q", result, expected)
	}
}

func TestLoadPromptTemplateNotFound(t *testing.T) {
	fsys := fstest.MapFS{}
	_, err := LoadPromptTemplate(fsys, "missing.md", nil)
	if err == nil {
		t.Error("expected error for missing template")
	}
}

func TestLoadPromptTemplateInvalidSyntax(t *testing.T) {
	fsys := fstest.MapFS{
		"bad.md": &fstest.MapFile{
			Data: []byte("{{.Unclosed"),
		},
	}
	_, err := LoadPromptTemplate(fsys, "bad.md", nil)
	if err == nil {
		t.Error("expected error for invalid template syntax")
	}
}

func TestPromptTemplateRenderError(t *testing.T) {
	fsys := fstest.MapFS{
		"test.md": &fstest.MapFile{
			Data: []byte("{{.Missing.Field}}"),
		},
	}

	pt, err := LoadPromptTemplate(fsys, "test.md", nil)
	if err != nil {
		t.Fatalf("LoadPromptTemplate failed: %v", err)
	}

	_, err = pt.Render(struct{}{})
	if err == nil {
		t.Error("expected error for missing field in template")
	}
}
