package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pgnds/request_counter/internal/naive"
)

func BenchmarkNaive(b *testing.B) {
	for _, n := range []int{1_000, 10_000, 50_000, 100_000, 500_000} {
		b.Run(fmt.Sprintf("timestamps=%d", n), func(b *testing.B) {
			file := filepath.Join(b.TempDir(), "naive.log")
			writeTimestamps(b, file, n)

			c, err := naive.New(file)
			if err != nil {
				b.Fatal(err)
			}
			server := httptest.NewServer(c)
			defer server.Close()

			var latencies []time.Duration
			start := time.Now()
			for b.Loop() {
				sent := time.Now()
				resp, err := http.Get(server.URL)
				if err != nil {
					b.Fatal(err)
				}
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					b.Fatalf("status %d", resp.StatusCode)
				}
				latencies = append(latencies, time.Since(sent))
			}
			elapsed := time.Since(start)

			slices.Sort(latencies)
			b.ReportMetric(float64(len(latencies))/elapsed.Seconds(), "req/s")
			b.ReportMetric(latencies[len(latencies)*90/100].Seconds()*1000, "p90(ms)")
		})
	}
}

func writeTimestamps(b *testing.B, file string, n int) {
	b.Helper()
	var sb strings.Builder
	start := time.Now().Unix() - 50
	for i := range n {
		fmt.Fprintln(&sb, start+int64(i*50/n))
	}
	if err := os.WriteFile(file, []byte(sb.String()), 0o644); err != nil {
		b.Fatal(err)
	}
}
