package jobrunner

import (
	"testing"
)

func TestExtractJSONArray(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "simple array",
			input: `[{"title": "Test", "url": "http://example.com", "summary": "A test"}]`,
			want:  `[{"title": "Test", "url": "http://example.com", "summary": "A test"}]`,
		},
		{
			name:  "with markdown code block",
			input: "```json\n[{\"title\": \"Test\"}]\n```",
			want:  `[{"title": "Test"}]`,
		},
		{
			name:  "with surrounding text",
			input: `Here are the articles I found:\n[{"title": "Test"}]\nHope this helps!`,
			want:  `[{"title": "Test"}]`,
		},
		{
			name:    "no array",
			input:   "No articles found.",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractJSONArray(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractJSONArray() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("extractJSONArray() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFixMalformedJSON(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "valid json unchanged",
			input: `[{"title": "Test"}]`,
			want:  `[{"title": "Test"}]`,
		},
		{
			name:  "embedded quote",
			input: `[{"title": "He said "hello" to me"}]`,
			want:  `[{"title": "He said \"hello\" to me"}]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fixMalformedJSON(tt.input)
			if got != tt.want {
				t.Errorf("fixMalformedJSON() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractArticlesJSON(t *testing.T) {
	input := `[{"title": "News Article", "url": "https://example.com/article", "summary": "This is a test."}]`
	
	articles, err := ExtractArticlesJSON(input)
	if err != nil {
		t.Fatalf("ExtractArticlesJSON() error = %v", err)
	}
	
	if len(articles) != 1 {
		t.Fatalf("expected 1 article, got %d", len(articles))
	}
	
	if articles[0].Title != "News Article" {
		t.Errorf("Title = %v, want %v", articles[0].Title, "News Article")
	}
	if articles[0].URL != "https://example.com/article" {
		t.Errorf("URL = %v, want %v", articles[0].URL, "https://example.com/article")
	}
}
