package parser

import (
	"regexp"
	"strings"
)

var digitsRe = regexp.MustCompile(`^\d+$`)
var ipRe = regexp.MustCompile(`^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$`)
var hexRe = regexp.MustCompile(`^[0-9a-fA-F]+$`)
var timestampRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}`)
var timeRe = regexp.MustCompile(`^\d{2}:\d{2}:\d{2}`)
var floatRe = regexp.MustCompile(`^\d+\.\d+$`)

type Token struct {
	Value string
	IsVar bool
}

type Template struct {
	ID      int
	Tokens  []Token
}

type Node struct {
	Children  map[string]*Node
	Depth     int
	ClusterID int
}

type DrainParser struct {
	Root      *Node
	Templates []*Template
	Depth     int
	MaxChild  int
	SimilarityThreshold float64
	NextID    int
}

func NewDrainParser(depth int, maxChild int, st float64) *DrainParser {
	return &DrainParser{
		Root:      &Node{Children: make(map[string]*Node)},
		Templates: nil,
		Depth:     depth,
		MaxChild:  maxChild,
		SimilarityThreshold: st,
		NextID:    1,
	}
}

func (dp *DrainParser) Train(lines []string) {
	for _, line := range lines {
		dp.AddLine(line)
	}
}

func (dp *DrainParser) AddLine(line string) int {
	toks := TokenizeContent(line)
	if len(toks) == 0 {
		return 0
	}
	cluster := dp.treeSearch(toks)
	if cluster == nil || similarity(cluster.Tokens, toks) < dp.SimilarityThreshold {
		tmpl := &Template{
			ID:     dp.NextID,
			Tokens: copyTokens(toks),
		}
		dp.NextID++
		dp.Templates = append(dp.Templates, tmpl)
		dp.addToTree(toks, len(dp.Templates)-1)
		return tmpl.ID
	}
	generalize(cluster, toks)
	return cluster.ID
}

func (dp *DrainParser) Match(line string) *Template {
	toks := TokenizeContent(line)
	if len(toks) == 0 {
		return nil
	}
	cluster := dp.treeSearch(toks)
	if cluster == nil || similarity(cluster.Tokens, toks) < dp.SimilarityThreshold {
		return nil
	}
	return cluster
}

func (dp *DrainParser) MatchWithVars(line string) (*Template, []string) {
	toks := TokenizeContent(line)
	if len(toks) == 0 {
		return nil, nil
	}
	cluster := dp.treeSearch(toks)
	if cluster == nil || similarity(cluster.Tokens, toks) < dp.SimilarityThreshold {
		return nil, nil
	}
	vars := ExtractVars(cluster.Tokens, toks)
	return cluster, vars
}

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
			node.Children[tok] = &Node{Children: make(map[string]*Node), Depth: i + 1, ClusterID: -1}
		}
		node = node.Children[tok]
	}
	if node.ClusterID == -1 {
		node.ClusterID = clusterIdx
	}
}

func TokenizeContent(content string) []Token {
	parts := splitContent(content)
	toks := make([]Token, len(parts))
	for i, p := range parts {
		toks[i] = Token{Value: p, IsVar: false}
	}
	return toks
}

func splitContent(s string) []string {
	parts := strings.Fields(s)
	if len(parts) == 0 {
		return nil
	}
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		result = append(result, p)
	}
	return result
}

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

func similarity(tmpl []Token, toks []Token) float64 {
	if len(tmpl) != len(toks) {
		return 0
	}
	match := 0
	for i := range tmpl {
		if tmpl[i].Value == toks[i].Value || tmpl[i].IsVar {
			match++
		}
	}
	return float64(match) / float64(len(toks))
}

func generalize(cluster *Template, toks []Token) {
	if len(cluster.Tokens) != len(toks) {
		return
	}
	for i := range cluster.Tokens {
		if cluster.Tokens[i].Value != toks[i].Value {
			if !cluster.Tokens[i].IsVar {
				cluster.Tokens[i].Value = "<*>"
				cluster.Tokens[i].IsVar = true
			}
		}
	}
}

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

func (t *Template) VarTokens() []Token {
	var vars []Token
	for _, tok := range t.Tokens {
		if tok.IsVar {
			vars = append(vars, tok)
		}
	}
	return vars
}

func copyTokens(toks []Token) []Token {
	c := make([]Token, len(toks))
	copy(c, toks)
	return c
}

func isTimestamp(s string) bool {
	return timestampRe.MatchString(s) || timeRe.MatchString(s)
}
