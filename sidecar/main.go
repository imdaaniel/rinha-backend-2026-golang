package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"rinha-backend-2026/golang/search"
)

func main() {
	refPath := os.Getenv("REFERENCES_PATH")
	if refPath == "" {
		candidates := []string{"./resources/references.json.gz", "/app/resources/references.json.gz", "../base/resources/references.json.gz", "./resources/references.json", "/app/resources/references.json", "../base/resources/references.json"}
		for _, candidate := range candidates {
			if _, err := os.Stat(candidate); err == nil {
				refPath = candidate
				break
			}
		}
		if refPath == "" {
			refPath = "data/references_example.json"
		}
	}

	if err := search.LoadReferencesAndBuild(refPath); err != nil {
		log.Fatalf("cannot load references: %v", err)
	}

	log.Printf("loaded %d references and built VP-tree index", search.ReferenceCount())

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

	refLabels, refDists := search.SearchIndex(req.Vector, req.K)
	resp := searchResponse{Labels: refLabels, Distances: refDists}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
