package parser

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type HeaderField struct {
	Index      int
	StringSubs uint8
	NumericSubs uint8
	Format     string
	StrLens    []int8
	NumLens    []int8
	Delim      string
}

type HeadFormat struct {
	HeadLength uint32
	IsMulti    bool
	HeadRegex  string
	Fields     []HeaderField
}

func (hf *HeadFormat) FormatHeader(fields []string) string {
	if len(fields) < len(hf.Fields) {
		return strings.Join(fields, " ")
	}
	var parts []string
	for i, f := range hf.Fields[:int(min(int(hf.HeadLength), len(fields)))] {
		part := formatField(f, fields[i])
		parts = append(parts, part)
	}
	return strings.Join(parts, "")
}

func formatField(f HeaderField, val string) string {
	if len(f.Format) == 0 {
		return val + f.Delim
	}
	formatted := fmt.Sprintf(f.Format, parseNumeric(val))
	return formatted + f.Delim
}

func parseNumeric(s string) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

type HeaderSchema struct {
	Fields  []HeaderField
}

func InferHeaderSchema(lines []string, headLen int) *HeadFormat {
	if len(lines) == 0 {
		return &HeadFormat{HeadLength: uint32(headLen)}
	}

	hf := &HeadFormat{
		HeadLength: uint32(headLen),
	}

	fieldValues := make([][]string, headLen)
	for i := range fieldValues {
		fieldValues[i] = nil
	}

	for _, line := range lines {
		parts := strings.Fields(line)
		for i := 0; i < headLen && i < len(parts); i++ {
			fieldValues[i] = append(fieldValues[i], parts[i])
		}
	}

	for i := 0; i < headLen; i++ {
		if len(fieldValues[i]) == 0 {
			continue
		}
		f := inferField(fieldValues[i])
		hf.Fields = append(hf.Fields, f)
	}

	return hf
}

func inferField(values []string) HeaderField {
	f := HeaderField{
		Index:  0,
		Delim:  " ",
	}

	allNumeric := true
	seen := make(map[string]bool)
	for _, v := range values {
		seen[v] = true
		if !isNumeric(v) {
			allNumeric = false
		}
	}

	if allNumeric {
		f.NumericSubs = 1
		f.Format = inferNumericFormat(values)
		f.NumLens = []int8{maxDigitWidth(values)}
	} else {
		f.StringSubs = 1
		f.Format = "%s"
		f.StrLens = []int8{variableLen}
	}

	return f
}

const variableLen int8 = -1

var formatDetectors = []struct {
	re      *regexp.Regexp
	format  string
}{
	{regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`), "%d-%d-%d"},
	{regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}`), "%s"},
	{regexp.MustCompile(`^\d{2}:\d{2}:\d{2}$`), "%d:%d:%d"},
	{regexp.MustCompile(`^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$`), "%s"},
	{regexp.MustCompile(`^\d+$`), "%d"},
	{regexp.MustCompile(`^\d+\.\d+$`), "%f"},
}

var FormatColors = map[string]string{
	"%d":     "0",
	"%f":     "1",
	"%s":     "2",
}

func inferNumericFormat(values []string) string {
	if len(values) == 0 {
		return "%d"
	}
	for _, d := range formatDetectors {
		if d.re.MatchString(values[0]) {
			return d.format
		}
	}
	return "%d"
}

func maxDigitWidth(values []string) int8 {
	max := int8(0)
	for _, v := range values {
		l := int8(len(v))
		if l > max {
			max = l
		}
	}
	return max
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func isNumeric(s string) bool {
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

func SplitHeader(line string, headLen int) (header []string, content string) {
	parts := strings.Fields(line)
	if len(parts) <= headLen {
		return parts, ""
	}
	return parts[:headLen], strings.Join(parts[headLen:], " ")
}

var widthPattern = regexp.MustCompile(`\d+`)

func (hf *HeadFormat) EstimateBitWidth() int {
	total := 0
	for _, f := range hf.Fields {
		if len(f.NumLens) > 0 {
			avg := 0
			for _, l := range f.NumLens {
				if l > 0 {
					avg += int(l)
				} else {
					avg += 8
				}
			}
			total += avg / max(1, len(f.NumLens))
		}
	}
	return total * int(hf.HeadLength) * 4
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

type LogLine struct {
	Header    []string
	Content   string
	Raw       string
	EID       int
	Variables []string
	Failed    bool
	MatchFailed bool
}

func (hf *HeadFormat) ReconstructHeader(fields []string) string {
	var b strings.Builder
	for i := 0; i < len(fields) && i < len(hf.Fields); i++ {
		b.WriteString(fields[i])
		if i < len(hf.Fields)-1 {
			b.WriteString(hf.Fields[i].Delim)
		}
	}
	return b.String()
}

func EltsortUnique(arr []string) []string {
	seen := make(map[string]struct{}, len(arr))
	j := 0
	for _, v := range arr {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		arr[j] = v
		j++
	}
	return arr[:j]
}
