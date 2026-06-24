package analyzer

type Relations struct {
	HeaderPatterns map[int]string
	VarPatterns    map[int]map[int]string
	HeaderDiff     map[int]bool
	VarDiff        map[int]map[int]bool
	HeaderDict     map[int]*DictInfo
	VarDict        map[int]map[int]*DictInfo
}

type DictInfo struct {
	Entries []string
	Mapping map[string]int
}

func NewRelations() *Relations {
	return &Relations{
		HeaderPatterns: make(map[int]string),
		VarPatterns:    make(map[int]map[int]string),
		HeaderDiff:     make(map[int]bool),
		VarDiff:        make(map[int]map[int]bool),
		HeaderDict:     make(map[int]*DictInfo),
		VarDict:        make(map[int]map[int]*DictInfo),
	}
}

const (
	MultiplicityThreshold     = 0.3
	DictEncodingThreshold     = 0.5
)

func AnalyzeColumn(stringColumn []string, multiplicityThreshold float64) (pattern string, subFields [][]string, allPos [][]uint8, isDict bool, dictEntries []string) {
	seen := make(map[string]int)
	for _, v := range stringColumn {
		seen[v]++
	}
	uniqueRatio := float64(len(seen)) / float64(len(stringColumn))

	if uniqueRatio < DictEncodingThreshold {
		entries := make([]string, 0, len(seen))
		for v := range seen {
			entries = append(entries, v)
		}
		return "", nil, nil, true, entries
	}

	if uniqueRatio > multiplicityThreshold {
		pat := DelimiterLCS(stringColumn)
		if pat != "" && len(pat) > 1 {
			ok, sf, pos := SplitByPattern(stringColumn, pat)
			if ok && len(sf[0]) > 1 {
				return pat, sf, pos, false, nil
			}
		}
	}

	return "", nil, nil, false, nil
}

func ShouldDeltaEncode(numericValues []int64) bool {
	if len(numericValues) < 2 {
		return false
	}
	deltas := DeltaEncode(numericValues)
	origEntropy := WeightedEntropyLogWeight(numericValues)
	deltaEntropy := WeightedEntropyLogWeight(deltas)
	return deltaEntropy <= origEntropy
}

func (r *Relations) AddHeaderPattern(colIdx int, pattern string) {
	r.HeaderPatterns[colIdx] = pattern
}

func (r *Relations) AddVarPattern(tid, colIdx int, pattern string) {
	if r.VarPatterns[tid] == nil {
		r.VarPatterns[tid] = make(map[int]string)
	}
	r.VarPatterns[tid][colIdx] = pattern
}

func (r *Relations) AddHeaderDiff(colIdx int) {
	r.HeaderDiff[colIdx] = true
}

func (r *Relations) AddVarDiff(tid, colIdx int) {
	if r.VarDiff[tid] == nil {
		r.VarDiff[tid] = make(map[int]bool)
	}
	r.VarDiff[tid][colIdx] = true
}

func (r *Relations) AddHeaderDict(colIdx int, entries []string) {
	r.HeaderDict[colIdx] = &DictInfo{
		Entries: entries,
		Mapping: make(map[string]int),
	}
	for i, e := range entries {
		r.HeaderDict[colIdx].Mapping[e] = i
	}
}

func (r *Relations) AddVarDict(tid, colIdx int, entries []string) {
	if r.VarDict[tid] == nil {
		r.VarDict[tid] = make(map[int]*DictInfo)
	}
	r.VarDict[tid][colIdx] = &DictInfo{
		Entries: entries,
		Mapping: make(map[string]int),
	}
	for i, e := range entries {
		r.VarDict[tid][colIdx].Mapping[e] = i
	}
}

func DeltaEncodeInt32(values []int32) []int32 {
	if len(values) == 0 {
		return nil
	}
	deltas := make([]int32, len(values))
	deltas[0] = values[0]
	for i := 1; i < len(values); i++ {
		deltas[i] = values[i] - values[i-1]
	}
	return deltas
}

func DeltaDecodeInt32(deltas []int32) []int32 {
	if len(deltas) == 0 {
		return nil
	}
	values := make([]int32, len(deltas))
	values[0] = deltas[0]
	for i := 1; i < len(deltas); i++ {
		values[i] = values[i-1] + deltas[i]
	}
	return values
}
