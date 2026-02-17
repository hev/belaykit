package claude

import (
	"errors"
	"testing"
)

func TestStripCodeFences(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no fences",
			input:    `{"key": "value"}`,
			expected: "{\"key\": \"value\"}\n",
		},
		{
			name:     "json fences",
			input:    "```json\n{\"key\": \"value\"}\n```",
			expected: "{\"key\": \"value\"}\n",
		},
		{
			name:     "plain fences",
			input:    "```\n{\"key\": \"value\"}\n```",
			expected: "{\"key\": \"value\"}\n",
		},
		{
			name:     "text around fenced json",
			input:    "Some text\n```json\n{\"key\": \"value\"}\n```\nMore text",
			expected: "Some text\n{\"key\": \"value\"}\nMore text\n",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "\n",
		},
		{
			name:     "fence with language tag",
			input:    "```javascript\nconsole.log('hi')\n```",
			expected: "console.log('hi')\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StripCodeFences(tt.input)
			if result != tt.expected {
				t.Errorf("got %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantKey string
		wantErr bool
	}{
		{
			name:    "clean json",
			input:   `{"key": "value"}`,
			wantKey: "value",
		},
		{
			name:    "json with surrounding text",
			input:   `Here is the result: {"key": "value"} done.`,
			wantKey: "value",
		},
		{
			name:    "json with code fences",
			input:   "```json\n{\"key\": \"value\"}\n```",
			wantKey: "value",
		},
		{
			name:    "nested json",
			input:   `{"key": "value", "nested": {"a": 1}}`,
			wantKey: "value",
		},
		{
			name:    "no json",
			input:   "no json here",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "only opening brace",
			input:   "just a { here",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var dst map[string]any
			err := ExtractJSON(tt.input, &dst)
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if got, ok := dst["key"].(string); !ok || got != tt.wantKey {
					t.Errorf("got key=%v, want %q", dst["key"], tt.wantKey)
				}
			}
		})
	}
}

func TestExtractJSON_ErrNoJSON(t *testing.T) {
	var dst map[string]any
	err := ExtractJSON("no json", &dst)
	if !errors.Is(err, ErrNoJSON) {
		t.Errorf("expected ErrNoJSON, got %v", err)
	}
}

func TestExtractJSONArray(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{
			name:  "clean array",
			input: `[1, 2, 3]`,
			want:  3,
		},
		{
			name:  "array with surrounding text",
			input: `Here are the results: [1, 2, 3] done.`,
			want:  3,
		},
		{
			name:  "array with code fences",
			input: "```json\n[1, 2, 3]\n```",
			want:  3,
		},
		{
			name:  "array of objects",
			input: `[{"a": 1}, {"a": 2}]`,
			want:  2,
		},
		{
			name:    "no array",
			input:   "no array here",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var dst []any
			err := ExtractJSONArray(tt.input, &dst)
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && len(dst) != tt.want {
				t.Errorf("got %d elements, want %d", len(dst), tt.want)
			}
		})
	}
}

func TestExtractJSONArray_ErrNoJSON(t *testing.T) {
	var dst []any
	err := ExtractJSONArray("no array", &dst)
	if !errors.Is(err, ErrNoJSON) {
		t.Errorf("expected ErrNoJSON, got %v", err)
	}
}
