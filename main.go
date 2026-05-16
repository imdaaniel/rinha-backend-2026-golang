package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

var (
	references []Reference
	sidecarURL = "http://localhost:9998/search"
)

func main() {
	// load sample references for local fallback and development.
	if err := loadReferences("data/references_example.json"); err != nil {
		log.Printf("warning: could not load references_example.json: %v", err)
	}

	if envURL := os.Getenv("SIDECAR_URL"); envURL != "" {
		sidecarURL = envURL
	}

	http.HandleFunc("/ready", readyHandler)
	http.HandleFunc("/fraud-score", fraudHandler)

	srv := &http.Server{
		Addr:              ":9999",
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Println("starting server on :9999")
	log.Printf("using sidecar url %s", sidecarURL)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

func readyHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func fraudHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var p Payload
	if err := json.Unmarshal(body, &p); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	vec := Vectorize(&p)

	score, err := SearchSidecarScore(vec, 5)
	if err != nil {
		log.Printf("warning: sidecar search failed: %v; falling back to local brute force", err)
		score = BruteForceKNNScore(vec, references, 5)
	}
	approved := score < 0.6

	resp := map[string]interface{}{"approved": approved, "fraud_score": score}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func SearchSidecarScore(q []float32, k int) (float32, error) {
	type request struct {
		Vector []float32 `json:"vector"`
		K      int       `json:"k"`
	}
	var requestBody = request{Vector: q, K: k}

	b, err := json.Marshal(requestBody)
	if err != nil {
		return 0, err
	}

	resp, err := http.Post(sidecarURL, "application/json", bytes.NewReader(b))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("sidecar returned %d: %s", resp.StatusCode, string(body))
	}

	type searchResponse struct {
		Labels []string `json:"labels"`
	}
	var result searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, err
	}

	frauds := 0
	for _, label := range result.Labels {
		if label == "fraud" {
			frauds++
		}
	}
	if len(result.Labels) == 0 {
		return 0, fmt.Errorf("sidecar returned zero labels")
	}

	return float32(frauds) / float32(len(result.Labels)), nil
}

func loadReferences(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	return dec.Decode(&references)
}
