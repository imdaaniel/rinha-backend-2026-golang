package search

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/fogfish/hnsw"
	"github.com/fogfish/hnsw/vector"
	surface "github.com/kshard/vector"
)

const (
	binaryMagic   = "RINH"
	binaryVersion = 1
)

type Reference struct {
	Vector []float32 `json:"vector"`
	Label  string    `json:"label"`
}

var (
	enableTiming = false
	timingFile   = "/tmp/timings.log"
	timingMutex  sync.Mutex

	references []Reference
	vpIndex    *vpNode
	hnswIndex  *HNSW
	fogfishIndex *hnsw.HNSW[vector.VF32]

	mappedData   []byte
	labelsData   []byte
	labelOffsets []uint32
	labelLens    []uint32
	vectorData   []byte
	numRefs      int
	vectorDim    int
)

func LoadReferences(path string) error {
	// Try to open as binary first
	if strings.HasSuffix(path, ".bin") {
		return LoadBinaryReferences(path)
	}

	// Otherwise load as JSON and convert
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	if strings.HasSuffix(path, ".gz") {
		gz, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return err
		}
		defer gz.Close()
		data, err = io.ReadAll(gz)
		if err != nil {
			return err
		}
	}

	var refs []Reference
	if err := json.Unmarshal(data, &refs); err != nil {
		return err
	}

	references = refs
	return nil
}

func ConvertJSONToBin(jsonPath, binPath string) error {
	if err := os.MkdirAll(filepath.Dir(binPath), 0o755); err != nil {
		return err
	}

	src, err := os.Open(jsonPath)
	if err != nil {
		return err
	}
	defer src.Close()

	var reader io.Reader = src
	if strings.HasSuffix(jsonPath, ".gz") {
		gz, err := gzip.NewReader(src)
		if err != nil {
			return err
		}
		defer gz.Close()
		reader = gz
	}

	dec := json.NewDecoder(reader)
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '[' {
		return fmt.Errorf("expected JSON array in %s", jsonPath)
	}

	outTmp, err := os.CreateTemp("", "references-bin-*.tmp")
	if err != nil {
		return err
	}
	defer func() {
		outTmp.Close()
		os.Remove(outTmp.Name())
	}()

	if _, err := outTmp.Write(make([]byte, 16)); err != nil {
		return err
	}

	var labelBuf bytes.Buffer
	vectorTmp, err := os.CreateTemp("", "references-vec-*.tmp")
	if err != nil {
		return err
	}
	defer func() {
		vectorTmp.Close()
		os.Remove(vectorTmp.Name())
	}()

	count := 0
	dim := 0
	var floatBuf [4]byte
	for dec.More() {
		var ref Reference
		if err := dec.Decode(&ref); err != nil {
			return err
		}
		if count == 0 {
			dim = len(ref.Vector)
			if dim == 0 {
				return fmt.Errorf("empty vector in %s", jsonPath)
			}
		} else if len(ref.Vector) != dim {
			return fmt.Errorf("inconsistent vector dimension at item %d", count)
		}

		labelBytes := []byte(ref.Label)
		if len(labelBytes) > math.MaxUint16 {
			return fmt.Errorf("label too long")
		}
		var lenBuf [2]byte
		binary.LittleEndian.PutUint16(lenBuf[:], uint16(len(labelBytes)))
		if _, err := labelBuf.Write(lenBuf[:]); err != nil {
			return err
		}
		if _, err := labelBuf.Write(labelBytes); err != nil {
			return err
		}

		for _, f := range ref.Vector {
			binary.LittleEndian.PutUint32(floatBuf[:], math.Float32bits(f))
			if _, err := vectorTmp.Write(floatBuf[:]); err != nil {
				return err
			}
		}
		count++
	}

	if _, err := dec.Token(); err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("no references found in %s", jsonPath)
	}

	labelDataSize := labelBuf.Len()
	if _, err := outTmp.Write(labelBuf.Bytes()); err != nil {
		return err
	}

	pad := (4 - ((16 + labelDataSize) % 4)) % 4
	if pad > 0 {
		if _, err := outTmp.Write(make([]byte, pad)); err != nil {
			return err
		}
	}

	if _, err := vectorTmp.Seek(0, 0); err != nil {
		return err
	}
	if _, err := io.Copy(outTmp, vectorTmp); err != nil {
		return err
	}

	var header [16]byte
	copy(header[:4], []byte(binaryMagic))
	binary.LittleEndian.PutUint16(header[4:6], binaryVersion)
	binary.LittleEndian.PutUint16(header[6:8], uint16(dim))
	binary.LittleEndian.PutUint32(header[8:12], uint32(count))
	binary.LittleEndian.PutUint32(header[12:16], uint32(labelDataSize))

	if _, err := outTmp.Seek(0, 0); err != nil {
		return err
	}
	if _, err := outTmp.Write(header[:]); err != nil {
		return err
	}
	if err := outTmp.Close(); err != nil {
		return err
	}

	return os.Rename(outTmp.Name(), binPath)
}

func LoadBinaryReferences(path string) error {
	data, err := mmapFile(path)
	if err != nil {
		return err
	}
	mappedData = data

	if len(data) < 16 {
		return fmt.Errorf("invalid binary index file")
	}
	if string(data[:4]) != binaryMagic {
		return fmt.Errorf("unsupported binary index format")
	}
	if ver := binary.LittleEndian.Uint16(data[4:6]); ver != binaryVersion {
		return fmt.Errorf("unsupported binary index version %d", ver)
	}
	vectorDim = int(binary.LittleEndian.Uint16(data[6:8]))
	numRefs = int(binary.LittleEndian.Uint32(data[8:12]))
	labelDataSize := int(binary.LittleEndian.Uint32(data[12:16]))

	labelSectionEnd := 16 + labelDataSize
	if labelSectionEnd > len(data) {
		return fmt.Errorf("invalid label section size")
	}

	labelsData = data[16:labelSectionEnd]
	labelOffsets = make([]uint32, numRefs)
	labelLens = make([]uint32, numRefs)
	pos := 0
	for i := 0; i < numRefs; i++ {
		if pos+2 > len(labelsData) {
			return fmt.Errorf("invalid labels section")
		}
		labelLen := int(binary.LittleEndian.Uint16(labelsData[pos : pos+2]))
		pos += 2
		if pos+labelLen > len(labelsData) {
			return fmt.Errorf("invalid label entry")
		}
		labelOffsets[i] = uint32(pos)
		labelLens[i] = uint32(labelLen)
		pos += labelLen
	}
	if pos != len(labelsData) {
		return fmt.Errorf("invalid label section trailing bytes")
	}

	vectorOffset := alignUp(labelSectionEnd, 4)
	expectedLen := numRefs * vectorDim * 4
	if vectorOffset+expectedLen > len(data) {
		return fmt.Errorf("invalid vector section size")
	}
	vectorData = data[vectorOffset : vectorOffset+expectedLen]

	references = nil
	vpIndex = nil
	hnswIndex = nil
	fogfishIndex = nil
	return nil
}

func BuildIndex() error {
	if refCount() == 0 {
		return fmt.Errorf("no references loaded")
	}

	// Use fogfish/hnsw library for proven HNSW implementation
	// Very aggressive parameters for maximum speed at cost of accuracy
 hnswStart := time.Now()
	fogfishIndex = hnsw.New(
		vector.SurfaceVF32(surface.Euclidean()),
		hnsw.WithEfConstruction(50), // Very reduced for faster construction
		hnsw.WithM(4),               // Very reduced for faster queries
		hnsw.WithM0(8),              // Very reduced for faster queries
	)
	logTiming("hnsw_creation", time.Since(hnswStart))
	
	// Add all vectors to HNSW
	// Pad vectors to 16 dimensions (multiple of 4) for SIMD compatibility
	insertStart := time.Now()
	for i := 0; i < refCount(); i++ {
		vec := vectorAt(i)
		paddedVec := make([]float32, 16)
		copy(paddedVec, vec)
		// Last 2 dimensions remain 0 (padding)
		fogfishIndex.Insert(vector.VF32{Key: uint32(i), Vec: paddedVec})
	}
	logTiming("hnsw_insert_all", time.Since(insertStart))

	return nil
}

func LoadReferencesAndBuild(path string) error {
	if err := LoadReferences(path); err != nil {
		return err
	}
	return BuildIndex()
}

func ReferenceCount() int {
	return refCount()
}

func SearchIndex(q []float32, k int) ([]string, []float32) {
	if k <= 0 {
		k = 5
	}
	if refCount() == 0 {
		return nil, nil
	}
	if k > refCount() {
		k = refCount()
	}
	if hnswIndex == nil {
		_ = BuildIndex()
	}

	// Use HNSW for search
	ids := hnswIndex.Search(q, k)
	if ids == nil || len(ids) == 0 {
		return BruteForceSearch(q, k)
	}

	labels := make([]string, len(ids))
	dists := make([]float32, len(ids))
	for i, id := range ids {
		labels[i] = objectLabel(id)
		dists[i] = distSq(q, objectVector(id))
	}
	return labels, dists
}

func SearchScore(q []float32, k int) (float32, error) {
	if k <= 0 {
		k = 5
	}
	if refCount() == 0 {
		return 0, fmt.Errorf("no references loaded")
	}
	if k > refCount() {
		k = refCount()
	}
	if fogfishIndex == nil {
		_ = BuildIndex()
	}

	// Use fogfish/hnsw library for search
	// Pad query vector to 16 dimensions for SIMD compatibility
	paddedQuery := make([]float32, 16)
	copy(paddedQuery, q)
	query := vector.VF32{Vec: paddedQuery}
	
	searchStart := time.Now()
	neighbors := fogfishIndex.Search(query, k, 5) // Very reduced for maximum speed
	logTiming("hnsw_search", time.Since(searchStart))
	
	if len(neighbors) == 0 {
		return 0, fmt.Errorf("no neighbors returned")
	}

	frauds := 0
	for _, neighbor := range neighbors {
		idx := int(neighbor.Key)
		if idx >= 0 && idx < refCount() && labelIsFraud(idx) {
			frauds++
		}
	}
	return float32(frauds) / float32(len(neighbors)), nil
}

func bruteForceSearchScore(q []float32, k int) (float32, error) {
	count := refCount()
	if count == 0 {
		return 0, fmt.Errorf("no references")
	}

	type pair struct {
		idx  int
		dist float32
	}

	// Use a small fixed-size array for top k to avoid allocations
	topK := make([]pair, k)
	for i := range topK {
		topK[i].dist = 1e9
	}

	for i := 0; i < count; i++ {
		d := distSq(q, vectorAt(i))
		// Find if this should be in top k
		maxIdx := 0
		for j := 1; j < k; j++ {
			if topK[j].dist > topK[maxIdx].dist {
				maxIdx = j
			}
		}
		if d < topK[maxIdx].dist {
			topK[maxIdx] = pair{idx: i, dist: d}
		}
	}

	frauds := 0
	for i := 0; i < k; i++ {
		if topK[i].idx < count && labelIsFraud(topK[i].idx) {
			frauds++
		}
	}
	return float32(frauds) / float32(k), nil
}

func labelIsFraud(idx int) bool {
	if len(references) > 0 {
		return references[idx].Label == "fraud"
	}
	off := labelOffsets[idx]
	label := labelsData[off : off+labelLens[idx]]
	return len(label) == 5 && label[0] == 'f' && label[1] == 'r' && label[2] == 'a' && label[3] == 'u' && label[4] == 'd'
}

func refCount() int {
	if len(references) > 0 {
		return len(references)
	}
	return numRefs
}

func objectVector(idx int) []float32 {
	if len(references) > 0 {
		return references[idx].Vector
	}
	return vectorAt(idx)
}

func objectLabel(idx int) string {
	if len(references) > 0 {
		return references[idx].Label
	}
	off := labelOffsets[idx]
	return string(labelsData[off : off+labelLens[idx]])
}

func vectorAt(idx int) []float32 {
	start := idx * vectorDim * 4
	end := start + vectorDim*4
	return bytesAsFloat32s(vectorData[start:end])
}

func bytesAsFloat32s(b []byte) []float32 {
	if len(b) == 0 {
		return nil
	}
	hdr := *(*reflect.SliceHeader)(unsafe.Pointer(&[]float32{}))
	hdr.Data = uintptr(unsafe.Pointer(&b[0]))
	hdr.Len = len(b) / 4
	hdr.Cap = len(b) / 4
	return *(*[]float32)(unsafe.Pointer(&hdr))
}

func alignUp(value, alignment int) int {
	return ((value + alignment - 1) / alignment) * alignment
}

func InitTiming(enabled bool, file string) {
	enableTiming = enabled
	if file != "" {
		timingFile = file
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
		return
	}
	defer f.Close()
	f.WriteString(line)
}
