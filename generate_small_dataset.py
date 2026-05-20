#!/usr/bin/env python3
"""
Generate a small dataset for performance testing.
This creates 1000 references instead of 3 million.
"""
import json
import random

# Generate 1000 references
references = []
for i in range(1000):
    # Generate random 14-dimensional vector
    vector = [random.random() for _ in range(14)]
    # Random label (30% fraud, 70% legit)
    label = "fraud" if random.random() < 0.3 else "legit"
    references.append({"vector": vector, "label": label})

# Write to JSON file
with open("data/references_small.json", "w") as f:
    json.dump(references, f)

print(f"Generated {len(references)} references in data/references_small.json")
