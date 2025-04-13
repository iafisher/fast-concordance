import math
import time
from pathlib import Path

pages = []
for p in Path("merged").glob("*/merged.txt"):
    pages.append(p.read_text())

start_time = time.perf_counter_ns()

n = 0
for page in pages:
    for c in page:
        if c == 'a':
            n += 1

duration_ns = time.perf_counter_ns() - start_time
duration_us = math.floor(duration_ns / 1000)

print(f"result:   {n}")
print(f"duration: {duration_us} us")
