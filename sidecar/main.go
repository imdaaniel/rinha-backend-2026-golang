package main

import (
	"encoding/json"
	"log"
	"math"
	"net/http"
	"os"
	"time"
)

type Reference struct {
	Vector []float32 `json:"vector"`
	Label  string    `json:"label"`
}

var (
	references []Reference
	index      *vpNode
)

func main() {
	refPath := os.Getenv("REFERENCES_PATH")
	if refPath == "" {
		refPath = "data/references_example.json"
	}

	if err := loadReferences(refPath); err != nil {
		log.Fatalf("cannot load references: %v", err)
	}

	index = BuildVPTree(references)
	log.Printf("loaded %d references and built VP-tree index", len(references))

	port := os.Getenv("SIDECAR_PORT")
	if port == "" {
		port = ":9998"
	}

	http.HandleFunc("/ready", readyHandler)
	http.HandleFunc("/search", searchHandler)

	srv := &http.Server{
		Addr:              port,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("starting sidecar on %s", port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("sidecar server error: %v", err)
	}
}

func readyHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

type searchRequest struct {
	Vector []float32 `json:"vector"`
	K      int       `json:"k"`
}

type searchResponse struct {
	Labels    []string  `json:"labels"`
	Distances []float32 `json:"distances"`
}

func searchHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req searchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if req.K <= 0 {
		req.K = 5
	}

	refLabels, refDists := SearchIndex(req.Vector, req.K)
	resp := searchResponse{Labels: refLabels, Distances: refDists}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func BruteForceSearch(q []float32, refs []Reference, k int) ([]string, []float32) {
	if len(refs) == 0 {
		return nil, nil
	}
	type pair struct {
		label string
		dist  float32
	}
	ps := make([]pair, 0, len(refs))
	for _, r := range refs {
		d := distSq(q, r.Vector)
		ps = append(ps, pair{label: r.Label, dist: d})
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
	var s float32
	for i := 0; i < n; i++ {
		dx := a[i] - b[i]
		s += dx * dx
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
