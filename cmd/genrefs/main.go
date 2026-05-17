package main

import (
	"fmt"
	"os"

	"rinha-backend-2026/golang/search"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintf(os.Stderr, "usage: genrefs <source.json.gz> <output.bin>\n")
		os.Exit(1)
	}

	src := os.Args[1]
	dst := os.Args[2]
	if err := search.ConvertJSONToBin(src, dst); err != nil {
		fmt.Fprintf(os.Stderr, "failed to generate binary references: %v\n", err)
		os.Exit(1)
	}
}
