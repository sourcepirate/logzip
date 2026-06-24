package parser

import (
	"testing"
)

func TestDrainParserBasic(t *testing.T) {
	lines := []string{
		"192.168.1.1 - - [10/Oct/2000:13:55:36 -0700] GET /index.html HTTP/1.0 200 2326",
		"192.168.1.2 - - [10/Oct/2000:13:55:37 -0700] GET /about.html HTTP/1.0 200 1234",
		"192.168.1.3 - - [10/Oct/2000:13:55:38 -0700] POST /api/login HTTP/1.0 401 512",
	}
	dp := NewDrainParser(8, 100, 0.6)
	dp.Train(lines)
	if len(dp.Templates) == 0 {
		t.Fatal("expected at least one template")
	}
	t.Logf("templates: %d", len(dp.Templates))
	for _, tmpl := range dp.Templates {
		t.Logf("template %d: %v", tmpl.ID, tmpl.Tokens)
	}
}

func TestDrainParserTemplates(t *testing.T) {
	lines := []string{
		"user alice logged in from 10.0.0.1",
		"user bob logged in from 10.0.0.2",
		"user charlie logged out",
		"user dave logged in from 10.0.0.3",
	}
	dp := NewDrainParser(4, 100, 0.5)
	dp.Train(lines)
	if len(dp.Templates) < 2 {
		t.Fatalf("expected at least 2 templates, got %d", len(dp.Templates))
	}
	foundLoggedOut := false
	for _, tmpl := range dp.Templates {
		for _, tok := range tmpl.Tokens {
			if tok.Value == "out" && !tok.IsVar {
				foundLoggedOut = true
			}
		}
	}
	if !foundLoggedOut {
		t.Errorf("expected a 'logged out' template to exist")
	}
}

func TestDrainParserMatch(t *testing.T) {
	lines := []string{
		"ERROR server timeout after 5000 from 10.0.0.1",
		"ERROR server timeout after 3000 from 10.0.0.2",
		"INFO  request completed in 42",
	}
	dp := NewDrainParser(8, 100, 0.5)
	dp.Train(lines)
	if len(dp.Templates) == 0 {
		t.Fatal("no templates")
	}
	tmpl, vars := dp.MatchWithVars("ERROR server timeout after 7000 from 10.0.0.3")
	if tmpl == nil {
		t.Fatal("expected match")
	}
	if len(vars) != 2 {
		t.Fatalf("expected 2 variables, got %d: %v", len(vars), vars)
	}
}

func TestHeaderInference(t *testing.T) {
	lines := []string{
		"192.168.1.1 - - [10/Oct/2000:13:55:36 -0700] GET /index.html HTTP/1.0 200 2326",
		"192.168.1.2 - - [10/Oct/2000:13:55:37 -0700] GET /about.html HTTP/1.0 200 1234",
		"192.168.1.3 - - [10/Oct/2000:13:55:38 -0700] POST /api/login HTTP/1.0 401 512",
	}
	hf := InferHeaderSchema(lines, 5)
	if hf.HeadLength != 5 {
		t.Fatalf("expected headLength 5, got %d", hf.HeadLength)
	}
	if len(hf.Fields) != 5 {
		t.Fatalf("expected 5 fields, got %d", len(hf.Fields))
	}
}

func TestSplitHeader(t *testing.T) {
	line := "192.168.1.1 - - [10/Oct/2000:13:55:36 -0700] GET /index.html HTTP/1.0 200 2326"
	header, content := SplitHeader(line, 5)
	if len(header) != 5 {
		t.Fatalf("expected 5 header fields, got %d: %v", len(header), header)
	}
	if len(content) == 0 {
		t.Fatal("expected non-empty content")
	}
	if content != "GET /index.html HTTP/1.0 200 2326" {
		t.Fatalf("unexpected content: %q, expected %q", content, "GET /index.html HTTP/1.0 200 2326")
	}
}

func TestPreprocess(t *testing.T) {
	cases := []struct {
		in  string
		out string
	}{
		{"hello", "hello"},
		{"12345", "<*>"},
		{"10.0.0.1", "<*>"},
		{"3.14", "<*>"},
		{"abc123", "abc123"},
		{"5000", "<*>"},
		{"ERROR", "ERROR"},
	}
	for _, c := range cases {
		got := preprocess(c.in)
		if got != c.out {
			t.Errorf("preprocess(%q) = %q, want %q", c.in, got, c.out)
		}
	}
}
