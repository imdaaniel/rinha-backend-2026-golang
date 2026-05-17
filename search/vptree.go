package search

import (
	"math"
	"sort"
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

func BuildVPTree(idxs []int) *vpNode {
	if len(idxs) == 0 {
		return nil
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
	pivotVec := objectVector(pivotIndex)
	for i, idx := range others {
		dists[i] = distSq(pivotVec, objectVector(idx))
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
			h.add(idx, distSq(q, objectVector(idx)))
		}
		return
	}

	pivotDist := distSq(q, objectVector(n.pivot))
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

func BruteForceSearch(q []float32, k int) ([]string, []float32) {
	count := refCount()
	if count == 0 {
		return nil, nil
	}
	type pair struct {
		label string
		dist  float32
	}
	ps := make([]pair, 0, count)
	for i := 0; i < count; i++ {
		d := distSq(q, objectVector(i))
		ps = append(ps, pair{label: objectLabel(i), dist: d})
	}

	for i := 0; i < len(ps); i++ {
		for j := i + 1; j < len(ps); j++ {
			if ps[j].dist < ps[i].dist {
				ps[i], ps[j] = ps[j], ps[i]
			}
		}
	}

	take := k
	if take > len(ps) {
		take = len(ps)
	}

	labels := make([]string, take)
	dists := make([]float32, take)
	for i := 0; i < take; i++ {
		labels[i] = ps[i].label
		dists[i] = ps[i].dist
	}

	return labels, dists
}

func distSq(a, b []float32) float32 {
	la := len(a)
	lb := len(b)
	n := la
	if lb < n {
		n = lb
	}
	
	// Loop unrolling for better performance
	var s float32
	i := 0
	// Process 4 elements at a time
	for i+4 <= n {
		dx0 := a[i] - b[i]
		dx1 := a[i+1] - b[i+1]
		dx2 := a[i+2] - b[i+2]
		dx3 := a[i+3] - b[i+3]
		s += dx0*dx0 + dx1*dx1 + dx2*dx2 + dx3*dx3
		i += 4
	}
	// Process remaining elements
	for i < n {
		dx := a[i] - b[i]
		s += dx * dx
		i++
	}
	
	if la != lb {
		var rem float32
		if la > lb {
			for i := lb; i < la; i++ {
				rem += a[i] * a[i]
			}
		} else {
			for i := la; i < lb; i++ {
				rem += b[i] * b[i]
			}
		}
		s += rem
	}
	if math.IsNaN(float64(s)) || math.IsInf(float64(s), 0) {
		return 1e9
	}
	return s
}
