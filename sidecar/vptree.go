package main

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"sort"
	"strings"
)

const leafSize = 64

type vpNode struct {
	pivot     int
	threshold float32
	left      *vpNode
	right     *vpNode
	indexes   []int
}

type neighbor struct {
	idx  int
	dist float32
}

type neighborSet struct {
	items []neighbor
	k     int
}

func newNeighborSet(k int) *neighborSet {
	return &neighborSet{k: k}
}

func (h *neighborSet) add(idx int, dist float32) {
	if h.k <= 0 {
		return
	}
	if len(h.items) < h.k {
		h.items = append(h.items, neighbor{idx: idx, dist: dist})
		if len(h.items) == h.k {
			sort.Slice(h.items, func(i, j int) bool {
				return h.items[i].dist > h.items[j].dist
			})
		}
		return
	}
	if dist >= h.items[0].dist {
		return
	}
	h.items[0] = neighbor{idx: idx, dist: dist}
	sort.Slice(h.items, func(i, j int) bool {
		return h.items[i].dist > h.items[j].dist
	})
}

func (h *neighborSet) maxDist() float32 {
	if len(h.items) < h.k {
		return 1e9
	}
	return h.items[0].dist
}

func (h *neighborSet) sortedAscending() []neighbor {
	sort.Slice(h.items, func(i, j int) bool {
		return h.items[i].dist < h.items[j].dist
	})
	return h.items
}

func BuildVPTree(refs []Reference) *vpNode {
	if len(refs) == 0 {
		return nil
	}
	idxs := make([]int, len(refs))
	for i := range refs {
		idxs[i] = i
	}
	return buildVPTree(idxs)
}

func buildVPTree(idxs []int) *vpNode {
	if len(idxs) <= leafSize {
		return &vpNode{indexes: append([]int(nil), idxs...)}
	}

	pivotIndex := idxs[len(idxs)-1]
	others := idxs[:len(idxs)-1]

	dists := make([]float32, len(others))
	pivotVec := references[pivotIndex].Vector
	for i, idx := range others {
		dists[i] = distSq(pivotVec, references[idx].Vector)
	}

	threshold := findMedian(dists)
	inside := make([]int, 0, len(others))
	outside := make([]int, 0, len(others))
	for i, idx := range others {
		if dists[i] <= threshold {
			inside = append(inside, idx)
		} else {
			outside = append(outside, idx)
		}
	}

	return &vpNode{
		pivot:     pivotIndex,
		threshold: threshold,
		left:      buildVPTree(inside),
		right:     buildVPTree(outside),
	}
}

func findMedian(values []float32) float32 {
	if len(values) == 0 {
		return 0
	}
	copied := append([]float32(nil), values...)
	sort.Slice(copied, func(i, j int) bool { return copied[i] < copied[j] })
	return copied[len(copied)/2]
}

func (n *vpNode) search(q []float32, h *neighborSet) {
	if n == nil {
		return
	}
	if n.indexes != nil {
		for _, idx := range n.indexes {
			h.add(idx, distSq(q, references[idx].Vector))
		}
		return
	}

	pivotDist := distSq(q, references[n.pivot].Vector)
	h.add(n.pivot, pivotDist)

	var near, far *vpNode
	if pivotDist < n.threshold {
		near = n.left
		far = n.right
	} else {
		near = n.right
		far = n.left
	}

	if near != nil {
		near.search(q, h)
	}

	if far != nil {
		if len(h.items) < h.k || abs32(pivotDist-n.threshold) <= h.maxDist() {
			far.search(q, h)
		}
	}
}

func abs32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

func SearchIndex(q []float32, k int) ([]string, []float32) {
	if k <= 0 {
		k = 5
	}
	if len(references) == 0 {
		return nil, nil
	}
	if k > len(references) {
		k = len(references)
	}
	if index == nil {
		return BruteForceSearch(q, references, k)
	}

	h := newNeighborSet(k)
	index.search(q, h)
	if len(h.items) == 0 {
		return BruteForceSearch(q, references, k)
	}

	neighbors := h.sortedAscending()
	labels := make([]string, len(neighbors))
	dists := make([]float32, len(neighbors))
	for i, n := range neighbors {
		labels[i] = references[n.idx].Label
		dists[i] = n.dist
	}
	return labels, dists
}

func loadReferences(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	reader := io.Reader(f)
	if strings.HasSuffix(path, ".gz") {
		gz, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer gz.Close()
		reader = gz
	}

	dec := json.NewDecoder(reader)
	return dec.Decode(&references)
}
