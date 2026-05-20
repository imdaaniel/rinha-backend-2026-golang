#!/usr/bin/env python3
"""
Analyze timing measurements to identify performance bottlenecks.
"""
import re
from collections import defaultdict

# Read timing file
timings = defaultdict(list)
with open("timings.log", "r") as f:
    for line in f:
        match = re.match(r".*\s(\w+)\s+(\d+)", line)
        if match:
            operation = match.group(1)
            duration = int(match.group(2))
            timings[operation].append(duration)

# Calculate statistics
print("Timing Analysis (microseconds)")
print("=" * 50)
for operation in sorted(timings.keys()):
    values = timings[operation]
    count = len(values)
    avg = sum(values) / count
    min_val = min(values)
    max_val = max(values)
    values_sorted = sorted(values)
    p50 = values_sorted[int(count * 0.5)]
    p95 = values_sorted[int(count * 0.95)]
    p99 = values_sorted[int(count * 0.99)]
    
    print(f"\n{operation}:")
    print(f"  Count: {count}")
    print(f"  Avg: {avg:.2f} µs")
    print(f"  Min: {min_val} µs")
    print(f"  Max: {max_val} µs")
    print(f"  P50: {p50} µs")
    print(f"  P95: {p95} µs")
    print(f"  P99: {p99} µs")

# Calculate percentage of total request time
if "total_request" in timings and "search" in timings:
    total_avg = sum(timings["total_request"]) / len(timings["total_request"])
    search_avg = sum(timings["search"]) / len(timings["search"])
    json_avg = sum(timings["json_unmarshal"]) / len(timings["json_unmarshal"])
    vectorize_avg = sum(timings["vectorize"]) / len(timings["vectorize"])
    hnsw_search_avg = sum(timings["hnsw_search"]) / len(timings["hnsw_search"])
    
    print("\n" + "=" * 50)
    print("Percentage of Total Request Time:")
    print(f"  JSON Unmarshal: {(json_avg/total_avg)*100:.1f}%")
    print(f"  Vectorize: {(vectorize_avg/total_avg)*100:.1f}%")
    print(f"  HNSW Search: {(hnsw_search_avg/total_avg)*100:.1f}%")
    print(f"  Search (including overhead): {(search_avg/total_avg)*100:.1f}%")
