# Benchmark results

2026-11-09 20:11, Darwin arm64

## Latency: push → post-pull command ran (10 trials each)

| Mode | Min | Median | Mean | Max |
|---|---|---|---|---|
| poll, interval 2s | 162ms | 1.459s | 1.264s | 2.05s |
| poll, interval 5s | 1.401s | 2.31s | 2.658s | 4.367s |
| poll, interval 10s | 3.047s | 6.766s | 6.417s | 9.361s |
| webhook | 90ms | 95ms | 94ms | 98ms |

## Idle cost: 30s with no pushes, one repo

| Mode | git pulls | Pulls per day (extrapolated) | Memory (RSS) |
|---|---|---|---|
| poll, interval 2s | 15 | 43200 | 11.6 MB |
| poll, interval 5s | 6 | 17280 | 10.5 MB |
| webhook | 0 | 0 | 8.1 MB |

## Cost of one check with nothing new (10 runs)

| Remote | Min | Median | Mean | Max |
|---|---|---|---|---|
| local bare repo | 44ms | 47ms | 47ms | 52ms |
| github.com over https (ls-remote) | 664ms | 703ms | 717ms | 800ms |
