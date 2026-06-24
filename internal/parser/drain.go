package parser

import (
	"regexp"
	"strings"
)

//
// Regex patterns for identifying variable-like tokens.
//
var digitsRe = regexp.MustCompile(`^\d+$`)
var ipRe = regexp.MustCompile(`^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$`)
var hexRe = regexp.MustCompile(`^[0-9a-fA-F]+$`)
var floatRe = regexp.MustCompile(`^\d+\.\d+$`)

// Token is a single word from a log line after whitespace splitting.
// IsVar marks tokens that represent variable data (wildcards) in templates.
type Token struct {
	Value string
	IsVar bool
}

// Template is a generalized log event pattern.
// Each position is either a literal string or a <*> wildcard (IsVar=true).
type Template struct {
	ID     int
	Tokens []Token
}

// Node is one level in the prefix tree (trie).
// Children map from a preprocessed token string to the next node.
// ClusterID indexes into DrainParser.Templates when this is a leaf.
type Node struct {
	Children  map[string]*Node
	Depth     int
	ClusterID int
}

// DrainParser clusters log content lines into templates using a prefix tree.
//
// How it works:
//
//  1. Preprocessing: tokens that look like variables (digits, IPs, hex)
//     are replaced with the sentinel string "<*>". This collapses e.g.
//     "200" and "404" into the same key so they navigate to the same node.
//
//  2. Tree search: starting from the root, each preprocessed token selects
//     a child. If the exact token isn't found, <*> is tried as a fallback.
//     The leaf node stores a reference to the matching Template.
//
//  3. Generalization: when a new line matches an existing template, tokens
//     that differ between the two are replaced with <*> wildcards. This
//     widens the template to cover both lines.
//
//  4. Similarity threshold: a match is only accepted if at least this
//     fraction of tokens agree (default 0.6 = 60%). Below that, the line
//     starts a new template.
type DrainParser struct {
	Root               *Node
	Templates          []*Template
	Depth              int
	MaxChild           int
	SimilarityThreshold float64
	NextID             int
}

// NewDrainParser creates a parser with the given tree depth, max children per
// node, and similarity threshold. Depth limits how many tokens are examined
// for clustering — deeper trees make finer distinctions.
func NewDrainParser(depth int, maxChild int, st float64) *DrainParser {
	return &DrainParser{
		Root:               &Node{Children: make(map[string]*Node)},
		Templates:          nil,
		Depth:              depth,
		MaxChild:           maxChild,
		SimilarityThreshold: st,
		NextID:             1,
	}
}

// Train processes each line through AddLine to build the template library.
func (dp *DrainParser) Train(lines []string) {
	for _, line := range lines {
		dp.AddLine(line)
	}
}

// AddLine tokenizes a log line and either merges it into an existing template
// or creates a new one. Returns the template ID assigned to this line.
//
// Three-phase logic:
//   - Tokenize the line content into tokens.
//   - Search the prefix tree for the best-matching template.
//   - If no close match exists, create a new template + tree path.
//   - Otherwise, generalize the existing template to cover this line.
func (dp *DrainParser) AddLine(line string) int {
	toks := TokenizeContent(line)
	if len(toks) == 0 {
		return 0
	}

	cluster := dp.treeSearch(toks)
	if cluster == nil || cluster.similarityWith(toks) < dp.SimilarityThreshold {
		return dp.createTemplate(toks)
	}

	cluster.generalize(toks)
	return cluster.ID
}

// Match finds the best-matching template for a log line, or nil if none found.
func (dp *DrainParser) Match(line string) *Template {
	toks := TokenizeContent(line)
	if len(toks) == 0 {
		return nil
	}
	cluster := dp.treeSearch(toks)
	if cluster == nil || cluster.similarityWith(toks) < dp.SimilarityThreshold {
		return nil
	}
	return cluster
}

// MatchWithVars is like Match but also returns the variable values extracted
// from the wildcard positions.
func (dp *DrainParser) MatchWithVars(line string) (*Template, []string) {
	toks := TokenizeContent(line)
	if len(toks) == 0 {
		return nil, nil
	}
	cluster := dp.treeSearch(toks)
	if cluster == nil || cluster.similarityWith(toks) < dp.SimilarityThreshold {
		return nil, nil
	}
	vars := ExtractVars(cluster.Tokens, toks)
	return cluster, vars
}

// createTemplate makes a new template from the given tokens, registers it,
// and inserts its path into the prefix tree.
func (dp *DrainParser) createTemplate(toks []Token) int {
	tmpl := &Template{
		ID:     dp.NextID,
		Tokens: copyTokens(toks),
	}
	dp.NextID++
	dp.Templates = append(dp.Templates, tmpl)
	dp.addToTree(toks, len(dp.Templates)-1)
	return tmpl.ID
}

// treeSearch walks the prefix tree following preprocessed token values.
// At each level it first tries an exact match, then falls back to <*>.
// Returns the template at the leaf, or nil if no path reaches a leaf.
func (dp *DrainParser) treeSearch(toks []Token) *Template {
	node := dp.Root
	for i := 0; i < len(toks) && i < dp.Depth; i++ {
		tok := preprocess(toks[i].Value)
		if child, ok := node.Children[tok]; ok {
			node = child
		} else if child, ok := node.Children["<*>"]; ok {
			node = child
		} else {
			return nil
		}
	}
	if node.ClusterID >= 0 && node.ClusterID < len(dp.Templates) {
		return dp.Templates[node.ClusterID]
	}
	return nil
}

// addToTree inserts a path for the given tokens into the prefix tree.
// It is the mirror of treeSearch: it creates nodes as it descends rather
// than following existing ones. If a node has reached MaxChild and the
// exact child doesn't exist, it falls back to the <*> wildcard path
// (or gives up if neither fits).
func (dp *DrainParser) addToTree(toks []Token, clusterIdx int) {
	node := dp.Root
	for i := 0; i < len(toks) && i < dp.Depth; i++ {
		tok := preprocess(toks[i].Value)

		if len(node.Children) >= dp.MaxChild && node.Children[tok] == nil {
			if wild, ok := node.Children["<*>"]; ok {
				node = wild
				continue
			}
			return
		}
		if _, ok := node.Children[tok]; !ok {
			node.Children[tok] = &Node{
				Children:  make(map[string]*Node),
				Depth:     i + 1,
				ClusterID: -1,
			}
		}
		node = node.Children[tok]
	}
	if node.ClusterID == -1 {
		node.ClusterID = clusterIdx
	}
}

// TokenizeContent splits a log content string into tokens by whitespace.
func TokenizeContent(content string) []Token {
	parts := strings.Fields(content)
	toks := make([]Token, len(parts))
	for i, p := range parts {
		toks[i] = Token{Value: p, IsVar: false}
	}
	return toks
}

// preprocess replaces variable-like token values with the wildcard "<*>".
// This collapses tokens like "200" and "404" into the same key so they
// route to the same node in the prefix tree, enabling generalization.
func preprocess(s string) string {
	if digitsRe.MatchString(s) || floatRe.MatchString(s) {
		return "<*>"
	}
	if ipRe.MatchString(s) {
		return "<*>"
	}
	if hexRe.MatchString(s) && len(s) >= 8 {
		return "<*>"
	}
	return s
}

// ExtractVars collects the actual values at wildcard positions by comparing
// a template against the concrete tokens from a log line.
//
// Example:
//
//	template: ["GET", "<*>", "HTTP/1.0", "<*>", "<*>"]
//	tokens:   ["GET", "/page", "HTTP/1.0", "200", "1234"]
//	result:   ["/page", "200", "1234"]
func ExtractVars(tmpl []Token, toks []Token) []string {
	if len(tmpl) != len(toks) {
		return nil
	}
	var vars []string
	for i := range tmpl {
		if tmpl[i].IsVar {
			vars = append(vars, toks[i].Value)
		}
	}
	return vars
}

// copyTokens returns a shallow copy of the token slice. New templates must
// own their copy because generalize mutates tokens in place, and we don't
// want aliasing between a template and its input lines.
func copyTokens(toks []Token) []Token {
	c := make([]Token, len(toks))
	copy(c, toks)
	return c
}

// similarityWith returns the fraction of tokens that match between this
// template and a concrete token sequence. A token matches if its value
// is identical OR if the template position is a wildcard (IsVar).
//
//	similarity = (# matching positions) / len(sequence)
//
// If 4 out of 5 tokens match, similarity = 0.8. The parser's threshold
// (default 0.6) requires at least 60% agreement for a match.
func (t *Template) similarityWith(toks []Token) float64 {
	if len(t.Tokens) != len(toks) {
		return 0
	}
	match := 0
	for i := range t.Tokens {
		if t.Tokens[i].Value == toks[i].Value || t.Tokens[i].IsVar {
			match++
		}
	}
	return float64(match) / float64(len(toks))
}

// generalize widens this template to cover the given token sequence.
// Positions where the two differ become wildcards (<*>, IsVar=true).
// Already-wildcard positions are left as-is.
//
// Example:
//
//	template: ["GET", "/about.html", "HTTP/1.0", "200", "1234"]
//	tokens:   ["GET", "/index.html", "HTTP/1.0", "200", "9999"]
//	result:   ["GET", "<*>",        "HTTP/1.0", "200", "<*>"]
func (t *Template) generalize(toks []Token) {
	if len(t.Tokens) != len(toks) {
		return
	}
	for i := range t.Tokens {
		if t.Tokens[i].Value != toks[i].Value && !t.Tokens[i].IsVar {
			t.Tokens[i].Value = "<*>"
			t.Tokens[i].IsVar = true
		}
	}
}

// VarTokens returns the subset of tokens that are wildcards, preserving
// their order. This is used to determine how many variable values a
// template expects per line.
func (t *Template) VarTokens() []Token {
	var vars []Token
	for _, tok := range t.Tokens {
		if tok.IsVar {
			vars = append(vars, tok)
		}
	}
	return vars
}
