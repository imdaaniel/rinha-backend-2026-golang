package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"
)

func main() {
	var (
		addr     = flag.String("addr", "http://localhost:9999/fraud-score", "target address")
		total    = flag.Int("n", 1000, "total requests")
		conc     = flag.Int("c", 50, "concurrency")
		filePath = flag.String("file", "", "path to JSON file with payloads")
	)
	flag.Parse()

	var payloads [][]byte
	var err error
	if *filePath != "" {
		payloads, err = readPayloads(*filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to read payload file: %v\n", err)
			os.Exit(1)
		}
		if len(payloads) == 0 {
			fmt.Fprintln(os.Stderr, "payload file contains no payloads")
			os.Exit(1)
		}
	} else {
		payloads = [][]byte{[]byte(`{"id":"tx-bench","transaction":{"amount":125.0,"installments":1,"requested_at":"2026-03-11T20:23:35Z"},"customer":{"avg_amount":100.0,"tx_count_24h":1,"known_merchants":["MERC-001"]},"merchant":{"id":"MERC-001","mcc":"5411","avg_amount":50.0},"terminal":{"is_online":false,"card_present":true,"km_from_home":5.0},"last_transaction":null}`)}
	}

	client := &http.Client{Timeout: 5 * time.Second}

	var wg sync.WaitGroup
	reqs := make(chan int)

	var mu sync.Mutex
	var latencies []float64
	var errors int

	start := time.Now()

	for i := 0; i < *conc; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range reqs {
				payload := payloads[idx%len(payloads)]
				t0 := time.Now()
				resp, err := client.Post(*addr, "application/json", bytes.NewReader(payload))
				d := time.Since(t0).Seconds() * 1000.0
				mu.Lock()
				if err != nil {
					errors++
				} else {
					if resp.StatusCode != http.StatusOK {
						errors++
					}
					resp.Body.Close()
					latencies = append(latencies, d)
				}
				mu.Unlock()
			}
		}()
	}

	for i := 0; i < *total; i++ {
		reqs <- i
	}
	close(reqs)
	wg.Wait()

	elapsed := time.Since(start).Seconds()
	totalDone := *total

	// analyze latencies
	sort.Float64s(latencies)
	p := func(q float64) float64 {
		if len(latencies) == 0 {
			return 0
		}
		idx := int(float64(len(latencies)-1) * q)
		return latencies[idx]
	}

	fmt.Printf("requests: %d, concurrency: %d, errors: %d, elapsed: %.2fs, rps: %.2f\n", totalDone, *conc, errors, elapsed, float64(totalDone)/elapsed)
	fmt.Printf("p50: %.2fms, p90: %.2fms, p99: %.2fms\n", p(0.50), p(0.90), p(0.99))
}

func readPayloads(path string) ([][]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}

	var payloads [][]byte
	if delim, ok := tok.(json.Delim); ok && delim == '[' {
		for dec.More() {
			var raw json.RawMessage
			if err := dec.Decode(&raw); err != nil {
				return nil, err
			}
			payloads = append(payloads, raw)
		}
		return payloads, nil
	}

	// Not an array: fallback to line-delimited JSON
	if err := f.Close(); err != nil {
		return nil, err
	}
	f, err = os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var raw json.RawMessage
		if err := json.Unmarshal(line, &raw); err != nil {
			return nil, err
		}
		payloads = append(payloads, raw)
	}
	return payloads, scanner.Err()
}
