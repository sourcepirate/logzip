package analyzer

import (
	"math"
	"strings"
	"unicode/utf8"
)

var delimiterUniverse = []rune{'-', '#', '>', '<', '_', ':', ';', ',', '[', ']', '\\', '/', '.', '(', ')'}

func IsDelimiter(r rune) bool {
	for _, d := range delimiterUniverse {
		if r == d {
			return true
		}
	}
	return false
}

func FilterDelimiters(s string) string {
	var b strings.Builder
	for _, r := range s {
		if IsDelimiter(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func LCS(a, b string) string {
	na := utf8.RuneCountInString(a)
	nb := utf8.RuneCountInString(b)

	ra := make([]rune, 0, na)
	for _, r := range a {
		ra = append(ra, r)
	}
	rb := make([]rune, 0, nb)
	for _, r := range b {
		rb = append(rb, r)
	}

	dp := make([][]int, na+1)
	for i := range dp {
		dp[i] = make([]int, nb+1)
	}

	for i := 1; i <= na; i++ {
		for j := 1; j <= nb; j++ {
			if ra[i-1] == rb[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else {
				if dp[i-1][j] > dp[i][j-1] {
					dp[i][j] = dp[i-1][j]
				} else {
					dp[i][j] = dp[i][j-1]
				}
			}
		}
	}

	lcsLen := dp[na][nb]
	result := make([]rune, lcsLen)
	i, j, idx := na, nb, lcsLen-1
	for i > 0 && j > 0 {
		if ra[i-1] == rb[j-1] {
			result[idx] = ra[i-1]
			i--
			j--
			idx--
		} else if dp[i-1][j] > dp[i][j-1] {
			i--
		} else {
			j--
		}
	}

	return string(result)
}

func DelimiterLCS(column []string) string {
	if len(column) == 0 {
		return ""
	}
	dstrings := make([]string, len(column))
	for i, s := range column {
		dstrings[i] = FilterDelimiters(s)
	}

	pat := dstrings[0]
	for _, ds := range dstrings[1:] {
		pat = LCS(pat, ds)
		if pat == "" {
			break
		}
		if utf8.RuneCountInString(pat) <= 1 {
			return ""
		}
	}
	return pat
}

func SplitByPattern(column []string, pattern string) (ok bool, subFields [][]string, allPos [][]uint8) {
	if pattern == "" {
		return false, nil, nil
	}

	subFields = make([][]string, len(column))
	allPos = make([][]uint8, len(column))
	firstCount := -1

	for i, s := range column {
		parts, pos := splitOnPattern(s, pattern)
		if firstCount < 0 {
			firstCount = len(parts)
		}
		if len(parts) != firstCount {
			return false, nil, nil
		}
		subFields[i] = parts
		allPos[i] = pos
	}

	return true, subFields, allPos
}

func splitOnPattern(s, pattern string) ([]string, []uint8) {
	if pattern == "" {
		return nil, nil
	}

	var parts []string
	var pos []uint8
	var runCount uint8

	i := 0
	patLen := len(pattern)
	patRunes := []rune(pattern)
	sRunes := []rune(s)

	for i < len(sRunes) {
		if i+patLen <= len(sRunes) {
			match := true
			for j := 0; j < patLen; j++ {
				if sRunes[i+j] != patRunes[j] {
					match = false
					break
				}
			}
			if match {
				runCount++
				i += patLen
				continue
			}
		}

		if runCount > 0 {
			pos = append(pos, runCount)
			runCount = 0
		}

		end := i + 1
		for end < len(sRunes) {
			foundPat := false
			if end+patLen <= len(sRunes) {
				match := true
				for j := 0; j < patLen; j++ {
					if sRunes[end+j] != patRunes[j] {
						match = false
						break
					}
				}
				if match {
					foundPat = true
				}
			}
			if foundPat {
				break
			}
			end++
		}

		parts = append(parts, string(sRunes[i:end]))
		i = end
	}

	if runCount > 0 {
		pos = append(pos, runCount)
	}

	return parts, pos
}

func RejoinByPattern(subFields []string, pos []uint8, pattern string) string {
	var b strings.Builder
	pRunes := []rune(pattern)
	for i, sf := range subFields {
		if i > 0 {
			b.WriteString(string(pRunes))
		}
		b.WriteString(sf)
	}
	return b.String()
}

func IsNumeric(s string) bool {
	if s == "" {
		return false
	}
	first := true
	hasDigit := false
	for _, r := range s {
		if first && (r == '-' || r == '+') {
			first = false
			continue
		}
		first = false
		if r == '.' {
			continue
		}
		if r >= '0' && r <= '9' {
			hasDigit = true
		} else {
			return false
		}
	}
	return hasDigit
}

func WeightedEntropy(values []int64) float64 {
	if len(values) == 0 {
		return 0
	}

	freq := make(map[int64]int)
	for _, v := range values {
		freq[v]++
	}
	n := float64(len(values))

	var h float64
	for _, count := range freq {
		p := float64(count) / n
		if p > 0 {
			h -= math.Log2(p) * p
		}
	}
	return h
}

func WeightedEntropyLogWeight(values []int64) float64 {
	if len(values) == 0 {
		return 0
	}

	freq := make(map[int64]int)
	for _, v := range values {
		freq[v]++
	}
	n := float64(len(values))

	var h float64
	for val, count := range freq {
		p := float64(count) / n
		w := math.Log2(float64(val + 1))
		if w < 1 {
			w = 1
		}
		if p > 0 {
			h -= w * math.Log2(p) * p
		}
	}
	return h
}

func DeltaEncode(values []int64) []int64 {
	if len(values) == 0 {
		return nil
	}
	deltas := make([]int64, len(values))
	deltas[0] = values[0]
	for i := 1; i < len(values); i++ {
		deltas[i] = values[i] - values[i-1]
	}
	return deltas
}

func DeltaDecode(deltas []int64) []int64 {
	if len(deltas) == 0 {
		return nil
	}
	values := make([]int64, len(deltas))
	values[0] = deltas[0]
	for i := 1; i < len(deltas); i++ {
		values[i] = values[i-1] + deltas[i]
	}
	return values
}
