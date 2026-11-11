# Benchmark results

2026-11-11 14:37, Darwin arm64

## Latency: push → post-pull command ran (10 trials each)

| Mode | Min | Median | Mean | Max |
|---|---|---|---|---|
| poll, interval 2s | 80ms | 1.033s | 902ms | 1.903s |
| poll, interval 5s | 143ms | 2.767s | 2.065s | 4.23s |
| poll, interval 10s | 507ms | 3.641s | 3.395s | 9.237s |
| webhook | 95ms | 97ms | 98ms | 104ms |

## Idle cost: 30s with no pushes, one repo

| Mode | git pulls | Pulls per day (extrapolated) | Memory (RSS) |
|---|---|---|---|
| poll, interval 2s | 15 | 43200 | 9.9 MB |
| poll, interval 5s | 6 | 17280 | 9.7 MB |
| webhook | 0 | 0 | 7.4 MB |

## Cost of one check with nothing new (10 runs)

| Remote | Min | Median | Mean | Max |
|---|---|---|---|---|
| local bare repo | 49ms | 51ms | 55ms | 91ms |
| github.com over https (ls-remote) | 687ms | 727ms | 751ms | 878ms |
