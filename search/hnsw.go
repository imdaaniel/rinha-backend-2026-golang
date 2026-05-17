package search

import (
	"math"
	"math/rand"
	"sort"
)

const (
	defaultM        = 16  // number of bi-directional links for each node
	defaultEfConst  = 200 // size of dynamic candidate list for construction
	defaultEfSearch = 50  // size of dynamic candidate list for search
)

type hnswNode struct {
	id       int
	vector   []float32
	neighbors [][]int // neighbors per level
}

type hnswLayer struct {
	nodes map[int]*hnswNode
}

type HNSW struct {
	dim        int
	M          int
	efConst    int
	efSearch   int
	entryPoint int
	maxLevel   int
	layers     []*hnswLayer
	rng        *rand.Rand
}

func NewHNSW(dim int) *HNSW {
	return &HNSW{
		dim:      dim,
		M:        defaultM,
		efConst:  defaultEfConst,
		efSearch: defaultEfSearch,
		layers:   []*hnswLayer{{nodes: make(map[int]*hnswNode)}},
		rng:      rand.New(rand.NewSource(42)),
	}
}

func (h *HNSW) Add(id int, vector []float32) {
	level := h.getRandomLevel()
	node := &hnswNode{
		id:       id,
		vector:   make([]float32, len(vector)),
		neighbors: make([][]int, level+1),
	}
	copy(node.vector, vector)

	// Ensure we have enough layers
	for len(h.layers) <= level {
		h.layers = append(h.layers, &hnswLayer{nodes: make(map[int]*hnswNode)})
	}

	// Add node to each level
	for l := 0; l <= level; l++ {
		h.layers[l].nodes[id] = node
	}

	// If this is the first node, set as entry point
	if h.entryPoint == -1 {
		h.entryPoint = id
		h.maxLevel = level
		return
	}

	// Find entry point for the top level
	ep := h.entryPoint
	for l := h.maxLevel; l > level; l-- {
		if ep == -1 {
			break
		}
		ep = h.searchLayerSingle(vector, ep, l)
	}

	// Insert at each level from top down
	for l := level; l >= 0; l-- {
		candidates := h.searchLayer(vector, ep, h.efConst, l)
		neighbors := h.selectNeighbors(candidates, h.M)
		
		// Add bidirectional links
		for _, neighborId := range neighbors {
			node.neighbors[l] = append(node.neighbors[l], neighborId)
			neighborNode := h.layers[l].nodes[neighborId]
			if len(neighborNode.neighbors[l]) < h.M {
				neighborNode.neighbors[l] = append(neighborNode.neighbors[l], id)
			} else {
				// If neighbor has too many connections, prune the worst one
				h.pruneConnections(neighborNode, l, id, vector)
			}
		}

		// Update entry point if we're at a higher level
		if l > h.maxLevel {
			h.maxLevel = l
			h.entryPoint = id
		}
		if len(candidates) > 0 {
			ep = candidates[0].id
		}
	}
}

func (h *HNSW) Search(query []float32, k int) []int {
	if h.entryPoint == -1 {
		return nil
	}

	// Search from top level down
	ep := h.entryPoint
	for l := h.maxLevel; l > 0; l-- {
		if ep == -1 {
			break
		}
		ep = h.searchLayerSingle(query, ep, l)
	}

	// Search at level 0 with efSearch
	candidates := h.searchLayer(query, ep, h.efSearch, 0)
	
	// Return top k results
	if k > len(candidates) {
		k = len(candidates)
	}
	
	result := make([]int, k)
	for i := 0; i < k; i++ {
		result[i] = candidates[i].id
	}
	return result
}

func (h *HNSW) searchLayerSingle(query []float32, entry int, level int) int {
	if entry == -1 {
		return -1
	}
	if level >= len(h.layers) {
		return -1
	}
	if h.layers[level].nodes[entry] == nil {
		return -1
	}

	current := entry
	currentDist := distSq(query, h.layers[level].nodes[current].vector)
	
	changed := true
	for changed {
		changed = false
		node := h.layers[level].nodes[current]
		for _, neighborId := range node.neighbors[level] {
			neighborNode := h.layers[level].nodes[neighborId]
			if neighborNode == nil {
				continue
			}
			ndist := distSq(query, neighborNode.vector)
			if ndist < currentDist {
				currentDist = ndist
				current = neighborId
				changed = true
			}
		}
	}
	
	return current
}

type candidate struct {
	id   int
	dist float32
}

func (h *HNSW) searchLayer(query []float32, entry int, ef int, level int) []candidate {
	if entry == -1 {
		return nil
	}
	if level >= len(h.layers) {
		return nil
	}
	if h.layers[level].nodes[entry] == nil {
		return nil
	}

	visited := make(map[int]bool)
	candidates := make([]candidate, 0, ef)
	w := make([]candidate, 0, ef)

	visited[entry] = true
	dist := distSq(query, h.layers[level].nodes[entry].vector)
	candidates = append(candidates, candidate{id: entry, dist: dist})
	w = append(w, candidate{id: entry, dist: dist})

	for len(w) > 0 {
		// Get closest element
		sort.Slice(w, func(i, j int) bool { return w[i].dist < w[j].dist })
		current := w[0]
		w = w[1:]

		// Check if we can stop
		if len(candidates) >= ef && current.dist > candidates[len(candidates)-1].dist {
			break
		}

		// Explore neighbors
		node := h.layers[level].nodes[current.id]
		for _, neighborId := range node.neighbors[level] {
			if visited[neighborId] {
				continue
			}
			visited[neighborId] = true
			
			neighborNode := h.layers[level].nodes[neighborId]
			ndist := distSq(query, neighborNode.vector)
			
			if len(candidates) < ef || ndist < candidates[len(candidates)-1].dist {
				candidates = append(candidates, candidate{id: neighborId, dist: ndist})
				sort.Slice(candidates, func(i, j int) bool { return candidates[i].dist < candidates[j].dist })
				if len(candidates) > ef {
					candidates = candidates[:ef]
				}
				w = append(w, candidate{id: neighborId, dist: ndist})
			}
		}
	}

	return candidates
}

func (h *HNSW) selectNeighbors(candidates []candidate, M int) []int {
	if len(candidates) <= M {
		result := make([]int, len(candidates))
		for i, c := range candidates {
			result[i] = c.id
		}
		return result
	}

	// Simple selection - take closest M
	result := make([]int, M)
	for i := 0; i < M; i++ {
		result[i] = candidates[i].id
	}
	return result
}

func (h *HNSW) pruneConnections(node *hnswNode, level int, newId int, newVector []float32) {
	// Find the worst connection to replace
	worstIdx := -1
	worstDist := float32(0)
	
	for i, neighborId := range node.neighbors[level] {
		neighborNode := h.layers[level].nodes[neighborId]
		dist := distSq(newVector, neighborNode.vector)
		if dist > worstDist {
			worstDist = dist
			worstIdx = i
		}
	}
	
	if worstIdx >= 0 {
		node.neighbors[level][worstIdx] = newId
	}
}

func (h *HNSW) getRandomLevel() int {
	level := 0
	for math.Float64frombits(h.rng.Uint64()) < 0.5 && level < h.M {
		level++
	}
	return level
}
