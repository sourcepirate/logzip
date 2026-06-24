package sampler

import (
	"math"
	"math/rand"
	"sort"
)

type SequenceSample struct {
	Indices   []int
	Centroids [][]float64
}

type ClusteringSampler struct {
	WindowH      int
	SampleRate   float64
	Candidates   int
	Threshold    float64
	TemplateNum  int
	Rand         *rand.Rand
}

func NewClusteringSampler(windowH int, sampleRate float64, candidates int, threshold float64) *ClusteringSampler {
	return &ClusteringSampler{
		WindowH:     windowH,
		SampleRate:  sampleRate,
		Candidates:  candidates,
		Threshold:   threshold,
		Rand:        rand.New(rand.NewSource(42)),
	}
}

type Sequence struct {
	EIDs    []int
	Index   int
}

func (cs *ClusteringSampler) Sample(sequences []Sequence) *SequenceSample {
	if len(sequences) == 0 {
		return &SequenceSample{}
	}

	vectors := cs.vectorize(sequences)
	cids, centers := cs.iterativeClustering(vectors, sequences)

	if len(centers) == 0 {
		idx := cs.Rand.Intn(len(sequences))
		return &SequenceSample{Indices: []int{sequences[idx].Index}}
	}

	balanced := cs.balancedSample(cids, sequences)
	return &SequenceSample{
		Indices:   balanced,
		Centroids: centers,
	}
}

func (cs *ClusteringSampler) vectorize(sequences []Sequence) [][]float64 {
	eidMax := 0
	for _, s := range sequences {
		for _, eid := range s.EIDs {
			if eid > eidMax {
				eidMax = eid
			}
		}
	}

	n := len(sequences)
	eidCount := eidMax + 1

	freq := make([][]float64, n)
	for i, s := range sequences {
		freq[i] = make([]float64, eidCount)
		for _, eid := range s.EIDs {
			if eid >= 0 && eid < eidCount {
				freq[i][eid]++
			}
		}
	}

	eidFreq := make([]int, eidCount)
	for _, s := range sequences {
		seen := make(map[int]bool)
		for _, eid := range s.EIDs {
			if eid >= 0 && !seen[eid] {
				eidFreq[eid]++
				seen[eid] = true
			}
		}
	}

	idf := make([]float64, eidCount)
	for e := 0; e < eidCount; e++ {
		cnt := eidFreq[e]
		if cnt == 0 {
			idf[e] = 0
			continue
		}
		idf[e] = math.Log(float64(n+1) / float64(cnt+1))
	}

	meanIDF := 0.0
	for _, v := range idf {
		meanIDF += v
	}
	if len(idf) > 0 {
		meanIDF /= float64(len(idf))
	}

	weights := make([]float64, eidCount)
	for e := 0; e < eidCount; e++ {
		x := idf[e] - meanIDF
		weights[e] = 1.0 / (1.0 + math.Exp(-x))
	}

	vectors := make([][]float64, n)
	for i := range vectors {
		vectors[i] = make([]float64, eidCount)
		for e := 0; e < eidCount; e++ {
			vectors[i][e] = freq[i][e] * weights[e]
		}
	}

	return vectors
}

func euclideanDist(a, b []float64) float64 {
	sum := 0.0
	for i := range a {
		d := a[i] - b[i]
		sum += d * d
	}
	return math.Sqrt(sum)
}

func (cs *ClusteringSampler) iterativeClustering(vectors [][]float64, sequences []Sequence) (map[int][]int, [][]float64) {
	n := len(vectors)
	mismatchIdx := make([]int, n)
	for i := range mismatchIdx {
		mismatchIdx[i] = i
	}

	allCids := make(map[int][]int)
	var centers [][]float64
	zeroRounds := 0

	sampleSize := int(float64(n) * cs.SampleRate)
	if sampleSize < cs.Candidates {
		sampleSize = cs.Candidates
	}
	if sampleSize > n {
		sampleSize = n
	}

	for zeroRounds <= 200 && len(mismatchIdx) > 1 {
		if len(mismatchIdx) <= sampleSize {
			break
		}

		cs.Rand.Shuffle(len(mismatchIdx), func(i, j int) {
			mismatchIdx[i], mismatchIdx[j] = mismatchIdx[j], mismatchIdx[i]
		})

		sampled := mismatchIdx[:sampleSize]
		sampledVectors := make([][]float64, len(sampled))
		for i, idx := range sampled {
			sampledVectors[i] = vectors[idx]
		}

		clusters := hacComplete(sampledVectors, cs.Threshold)
		if len(clusters) == 0 {
			zeroRounds++
			continue
		}

		var newCenters [][]float64
		for _, cl := range clusters {
			if len(cl) < 3 {
				continue
			}
			cent := meanVector(cl, sampledVectors)
			newCenters = append(newCenters, cent)
		}

		if len(newCenters) == 0 {
			zeroRounds++
			continue
		}

		centers = append(centers, newCenters...)

		var stillMismatch []int
		for _, idx := range mismatchIdx {
			v := vectors[idx]
			bestDist := cs.Threshold
			bestC := -1
			for ci, cent := range centers {
				d := euclideanDist(v, cent)
				if d < bestDist {
					bestDist = d
					bestC = ci
				}
			}
			if bestC >= 0 {
				allCids[bestC] = append(allCids[bestC], idx)
			} else {
				stillMismatch = append(stillMismatch, idx)
			}
		}

		if len(stillMismatch) == len(mismatchIdx) {
			zeroRounds++
		}
		mismatchIdx = stillMismatch

		if len(mismatchIdx) <= sampleSize {
			break
		}
	}

	for _, idx := range mismatchIdx {
		bestDist := cs.Threshold
		bestC := -1
		for ci, cent := range centers {
			d := euclideanDist(vectors[idx], cent)
			if d < bestDist {
				bestDist = d
				bestC = ci
			}
		}
		if bestC >= 0 {
			allCids[bestC] = append(allCids[bestC], idx)
		}
	}

	return allCids, centers
}

func meanVector(indices []int, vectors [][]float64) []float64 {
	if len(indices) == 0 || len(vectors) == 0 {
		return nil
	}
	n := len(indices)
	dim := len(vectors[0])
	mean := make([]float64, dim)
	for _, idx := range indices {
		for d := 0; d < dim; d++ {
			mean[d] += vectors[idx][d]
		}
	}
	for d := 0; d < dim; d++ {
		mean[d] /= float64(n)
	}
	return mean
}

func hacComplete(vectors [][]float64, threshold float64) [][]int {
	n := len(vectors)
	if n == 0 {
		return nil
	}

	dist := make([][]float64, n)
	for i := range dist {
		dist[i] = make([]float64, n)
		for j := i + 1; j < n; j++ {
			d := euclideanDist(vectors[i], vectors[j])
			dist[i][j] = d
			dist[j][i] = d
		}
	}

	clusters := make([][]int, n)
	for i := range clusters {
		clusters[i] = []int{i}
	}

	active := make([]bool, n)
	for i := range active {
		active[i] = true
	}
	activeCount := n

	for activeCount > 1 {
		minDist := math.MaxFloat64
		ci, cj := -1, -1
		for i := 0; i < n; i++ {
			if !active[i] {
				continue
			}
			for j := i + 1; j < n; j++ {
				if !active[j] {
					continue
				}
				d := clusterCompleteDist(i, j, clusters, dist)
				if d < minDist {
					minDist = d
					ci, cj = i, j
				}
			}
		}

		if minDist > threshold || ci < 0 {
			break
		}

		clusters[ci] = append(clusters[ci], clusters[cj]...)
		active[cj] = false
		activeCount--
	}

	var result [][]int
	for i := 0; i < n; i++ {
		if active[i] {
			result = append(result, clusters[i])
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return len(result[i]) > len(result[j])
	})

	return result
}

func clusterCompleteDist(i, j int, clusters [][]int, dist [][]float64) float64 {
	maxDist := 0.0
	for _, a := range clusters[i] {
		for _, b := range clusters[j] {
			if dist[a][b] > maxDist {
				maxDist = dist[a][b]
			}
		}
	}
	return maxDist
}

func (cs *ClusteringSampler) balancedSample(cids map[int][]int, sequences []Sequence) []int {
	if len(cids) == 0 {
		idx := cs.Rand.Intn(len(sequences))
		return []int{idx}
	}

	k := len(cids)
	m := cs.Candidates
	perCluster := (m + k - 1) / k
	if perCluster < 1 {
		perCluster = 1
	}

	var result []int
	for _, members := range cids {
		if len(members) == 0 {
			continue
		}
		n := perCluster
		if n > len(members) {
			n = len(members)
		}
		shuffled := make([]int, len(members))
		copy(shuffled, members)
		cs.Rand.Shuffle(len(shuffled), func(i, j int) {
			shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
		})
		for _, idx := range shuffled[:n] {
			result = append(result, idx)
		}
	}

	return result
}
