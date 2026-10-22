# request_counter

An HTTP server that answers every request with the number of requests it has
received in the last 60 seconds. Request timestamps are saved to
`request_timestamps.log`, so the count survives a restart.

## Running

```
go run .
```

The server listens on port 80.

## Tests

```
go test ./...
```

## Benchmark

`BenchmarkServer` starts the server the same way `main` does (real clock, real
file writes) and sends it real HTTP requests at a fixed rate for 10 seconds.
Each rate starts with a full minute of request history, as if the server had
already been running.

```
go test -run '^$' -bench .
```

All four rates take about 40 seconds.

### Results

Apple M2, Go 1.27.3:

| Rate | Answered/s | p50 | p99 | Queued writes | Unanswered |
|---|---|---|---|---|---|
| 10/s | 10 | 0.87 ms | 3.9 ms | 0 | 0 |
| 100/s | 100 | 0.57 ms | 1.8 ms | 0 | 0 |
| 1,000/s | 1,000 | 0.12 ms | 0.24 ms | 7,546 | 0 |
| 10,000/s | 1,022 | 0.04 ms | 4,900 ms | 10,000 (full) | 89,777 |

- **Answered/s**: requests answered per second, to compare with the rate sent.
- **p50 / p99**: latency of the requests that were answered.
- **Queued writes**: file writes still waiting when sending stopped. The queue
  holds 10,000 before requests start to block.
- **Unanswered**: requests with no response within a second of sending
  stopping.

Every request rewrites the whole log file, so each write gets slower as the
window fills up. At 1,000/s responses are still fast, but 7,546 writes are
already waiting: the server is falling behind and would start to stall a few
seconds later. Set `benchmarkDuration` in `main_test.go` to 20 seconds to see
the stalls directly. At 10,000/s the queue is full, and most requests go
unanswered.
