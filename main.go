package main

import (
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/valyala/fasthttp"

	jsoniter "github.com/json-iterator/go"

	"rinha-backend-2026/golang/search"
)

var (
	REFERENCES_PATH = os.Getenv("REFERENCES_PATH")
	SIDECAR_URL     = os.Getenv("SIDECAR_URL")
	sidecarURL      = ""
	vectorPool = sync.Pool{
		New: func() interface{} {
			return make([]float32, 14)
		},
	}
	jsonFast = jsoniter.ConfigFastest
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
		log.Printf("warning: could not load or build references from %s: %v", refPath, err)
		if err := search.LoadReferencesAndBuild("data/references_example.json"); err != nil {
			log.Fatalf("cannot load fallback references: %v", err)
		}
	}
	log.Printf("loaded %d references and built local index", search.ReferenceCount())

	if envURL := os.Getenv("SIDECAR_URL"); envURL != "" {
		sidecarURL = envURL
	}

	// Use fasthttp for extreme performance
	if err := fasthttp.ListenAndServe(":9999", requestHandler); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func requestHandler(ctx *fasthttp.RequestCtx) {
	path := string(ctx.Path())
	
	if path == "/ready" {
		// Only return ok if index is loaded and ready
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

		body := ctx.PostBody()
		if len(body) > 8192 {
			ctx.Error("bad request", fasthttp.StatusBadRequest)
			return
		}

		var p Payload
		if err := jsonFast.Unmarshal(body, &p); err != nil {
			ctx.Error("invalid json", fasthttp.StatusBadRequest)
			return
		}

		vec := vectorPool.Get().([]float32)
		defer vectorPool.Put(vec)

		VectorizeTo(&p, vec)

		score, err := search.SearchScore(vec, 5)
		if err != nil {
			ctx.Error("internal error", fasthttp.StatusInternalServerError)
			return
		}
		approved := score < 0.6

		resp := fmt.Sprintf(`{"approved":%t,"fraud_score":%f}`, approved, score)
		ctx.SetContentType("application/json")
		ctx.SetBodyString(resp)
		return
	}
	
	ctx.Error("not found", fasthttp.StatusNotFound)
}
