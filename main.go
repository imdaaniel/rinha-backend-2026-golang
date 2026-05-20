package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/valyala/fasthttp"

	jsoniter "github.com/json-iterator/go"

	"rinha-backend-2026/golang/search"
)

var (
	REFERENCES_PATH = os.Getenv("REFERENCES_PATH")
	SIDECAR_URL     = os.Getenv("SIDECAR_URL")
	sidecarURL      = ""
	enableTiming    = false
	timingFile      = os.Getenv("TIMING_FILE")
	timingMutex     sync.Mutex
	vectorPool = sync.Pool{
		New: func() interface{} {
			return make([]float32, 16)
		},
	}
	ready    atomic.Bool
	jsonFast = jsoniter.ConfigFastest
)

func main() {
	_ = os.WriteFile("/tmp/main-started", []byte(time.Now().Format(time.RFC3339)), 0644)
	refPath := os.Getenv("REFERENCES_PATH")
	if refPath == "" {
		candidates := []string{"/app/data/references.bin", "./data/references.bin", "./resources/references.json.gz", "/app/resources/references.json.gz", "../base/resources/references.json.gz", "./resources/references.json", "/app/resources/references.json", "../base/resources/references.json"}
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

	if err := search.LoadReferences(refPath); err != nil {
		log.Printf("warning: could not load references from %s: %v", refPath, err)
		if err := search.LoadReferences("data/references_example.json"); err != nil {
			log.Fatalf("cannot load fallback references: %v", err)
		}
	}
	_ = os.WriteFile("/tmp/refs-loaded", []byte(time.Now().Format(time.RFC3339)), 0644)

	log.Printf("loaded %d references (index not yet built)", search.ReferenceCount())

	if envURL := os.Getenv("SIDECAR_URL"); envURL != "" {
		sidecarURL = envURL
	}
	enableTiming = envBool("TIMING_ENABLED", false)
	if timingFile == "" {
		timingFile = "/tmp/timings.log"
	}
	search.InitTiming(enableTiming, timingFile)

	// Use fasthttp for extreme performance
	// Bind explicitly to all interfaces so reverse proxies can reach the service
	addr := "0.0.0.0:9999"
	log.Printf("starting server (background), listening on %s", addr)
	ln, err := net.Listen("tcp4", addr)
	if err != nil {
		_ = os.WriteFile("/tmp/listen-error", []byte(err.Error()), 0644)
		log.Fatalf("failed to bind %s: %v", addr, err)
	}
	_ = os.WriteFile("/tmp/listen-ok", []byte(addr), 0644)
	go func() {
		if err := fasthttp.Serve(ln, requestHandler); err != nil {
			_ = os.WriteFile("/tmp/server-serve-error", []byte(err.Error()), 0644)
			log.Fatalf("server serve error on %s: %v", addr, err)
		}
	}()

	// Now build index (may be slow) while server already accepts health checks
	_ = os.WriteFile("/tmp/index-building", []byte(time.Now().Format(time.RFC3339)), 0644)
	indexBuildStart := time.Now()
	if err := search.BuildIndex(); err != nil {
		log.Fatalf("failed building index: %v", err)
	}
	indexBuildDuration := time.Since(indexBuildStart)
	logTiming("index_build", indexBuildDuration)
	ready.Store(true)
	_ = os.WriteFile("/tmp/index-built", []byte(time.Now().Format(time.RFC3339)), 0644)
	log.Printf("index build complete")
	
	// Keep main process running
	select {}
}

func requestHandler(ctx *fasthttp.RequestCtx) {
	path := string(ctx.Path())
	
	if path == "/ready" {
		// Return ok if references are loaded (index may still be building)
		if search.ReferenceCount() > 0 {
			ctx.SetStatusCode(fasthttp.StatusOK)
			ctx.SetBodyString("ok")
		} else {
			ctx.SetStatusCode(fasthttp.StatusServiceUnavailable)
			ctx.SetBodyString("not ready")
		}
		return
	}
	
	if path == "/fraud-score" {
		if string(ctx.Method()) != "POST" {
			ctx.Error("method not allowed", fasthttp.StatusMethodNotAllowed)
			return
		}

		requestStart := time.Now()

		body := ctx.PostBody()
		if len(body) > 8192 {
			ctx.Error("bad request", fasthttp.StatusBadRequest)
			return
		}

		var p Payload
		unmarshalStart := time.Now()
		if err := jsonFast.Unmarshal(body, &p); err != nil {
			ctx.Error("invalid json", fasthttp.StatusBadRequest)
			return
		}
		logTiming("json_unmarshal", time.Since(unmarshalStart))

		vec := vectorPool.Get().([]float32)
		defer vectorPool.Put(vec)

		vectorizeStart := time.Now()
		VectorizeTo(&p, vec)
		logTiming("vectorize", time.Since(vectorizeStart))

		searchStart := time.Now()
		score, err := search.SearchScore(vec, 5)
		logTiming("search", time.Since(searchStart))
		if err != nil {
			ctx.Error("internal error", fasthttp.StatusInternalServerError)
			return
		}
		approved := score < 0.6

		resp := fmt.Sprintf(`{"approved":%t,"fraud_score":%f}`, approved, score)
		ctx.SetContentType("application/json")
		ctx.SetBodyString(resp)
		logTiming("total_request", time.Since(requestStart))
		return
	}
	
	ctx.Error("not found", fasthttp.StatusNotFound)
}

func envBool(key string, defaultValue bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return defaultValue
	}
	switch v {
	case "1", "true", "TRUE", "True", "yes", "YES", "Yes":
		return true
	case "0", "false", "FALSE", "False", "no", "NO", "No":
		return false
	default:
		return defaultValue
	}
}

func logTiming(operation string, duration time.Duration) {
	if !enableTiming {
		return
	}
	timingMutex.Lock()
	defer timingMutex.Unlock()
	timestamp := time.Now().Format(time.RFC3339Nano)
	line := fmt.Sprintf("%s %s %d\n", timestamp, operation, duration.Microseconds())
	f, err := os.OpenFile(timingFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("error opening timing file: %v", err)
		return
	}
	defer f.Close()
	if _, err := f.WriteString(line); err != nil {
		log.Printf("error writing timing: %v", err)
	}
}
