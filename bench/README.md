Benchmark harness

Build and run:

```bash
cd golang/bench
go build -o bench.exe
./bench.exe -n 1000 -c 50 -file ../../base/resources/example-payloads.json
```

Flags:
- `-n` total requests (default 1000)
- `-c` concurrency (default 50)
- `-addr` target URL (default http://localhost:9999/fraud-score)
- `-file` path to a JSON file with payloads (array format or line-delimited JSON)

Example with the provided test payloads:

```bash
./bench.exe -n 500 -c 20 -file ../../base/resources/example-payloads.json
```
