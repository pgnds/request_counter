package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"
)

type FakeTimer struct {
	currentTime time.Time
}

func (ft FakeTimer) Now() time.Time {
	return time.Now()
}

func (ft *FakeTimer) Set(newTime time.Time) {
	ft.currentTime = newTime
}

func TestHandler(t *testing.T) {
	fakeTimer := FakeTimer{currentTime: time.Now()}

	rc := RequestCounter{
		requests:              []int64{},
		latestCurrentRequests: make(chan []int64, 10_000),
		getNowTime:            fakeTimer.Now}

	responseRecorder := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "/", nil)
		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(rc.HandleRequest)
		handler.ServeHTTP(rr, req)
		return rr
	}

	// this will overwrite FILE_NAME in package
	FILE_NAME = "request_timestamps.temp1.log"
	defer os.Remove("request_timestamps.temp1.log")

	responseCount := responseRecorder().Body.String()
	if responseCount != "1" {
		t.Errorf("expected 1 requests, got %v", responseCount)
	}
	responseCount = responseRecorder().Body.String()
	if responseCount != "2" {
		t.Errorf("expected 2 requests, got %v", responseCount)
	}
	responseCount = responseRecorder().Body.String()
	if responseCount != "3" {
		t.Errorf("expected 3 requests, got %v", responseCount)
	}
}

func TestHandlerAfter60Seconds(t *testing.T) {
	fakeTimer := FakeTimer{currentTime: time.Now()}

	rc := RequestCounter{
		requests:              []int64{},
		latestCurrentRequests: make(chan []int64, 10_000),
		getNowTime:            fakeTimer.Now}

	responseRecorder := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "/", nil)
		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(rc.HandleRequest)
		handler.ServeHTTP(rr, req)
		return rr
	}

	// this will overwrite FILE_NAME in package
	FILE_NAME = "request_timestamps.temp1.log"
	defer os.Remove("request_timestamps.temp1.log")

	responseCount := responseRecorder().Body.String()
	if responseCount != "1" {
		t.Errorf("expected 1 requests, got %v", responseCount)
	}
	responseCount = responseRecorder().Body.String()
	if responseCount != "2" {
		t.Errorf("expected 2 requests, got %v", responseCount)
	}

	rc.getNowTime = func() time.Time { return time.Now().Add(65 * time.Second) }
	responseCount = responseRecorder().Body.String()
	if responseCount != "1" {
		t.Errorf("expected 1 requests, got %v", responseCount)
	}
}

func TestHandlerWithPreExistingEntries(t *testing.T) {
	filename := "request_timestamps.temp2.log"
	// this will overwrite FILE_NAME in package
	FILE_NAME = filename
	defer os.Remove(filename)

	fakeTimer := FakeTimer{currentTime: time.Now()}

	rc := RequestCounter{requests: []int64{
		time.Now().Add(-90 * time.Second).Unix(),
		time.Now().Add(-62 * time.Second).Unix(),
		time.Now().Add(-30 * time.Second).Unix(),
		time.Now().Add(-10 * time.Second).Unix()},
		latestCurrentRequests: make(chan []int64, 10_000),
		getNowTime:            fakeTimer.Now,
	}

	responseRecorder := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "/", nil)
		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(rc.HandleRequest)
		handler.ServeHTTP(rr, req)
		return rr
	}

	responseCount := responseRecorder().Body.String()
	if responseCount != "3" {
		t.Errorf("expected 3 requests, got %v", responseCount)
	}
}

func TestReadTimestampsFromFile(t *testing.T) {
	filename := "testReadTimestampsFromFile.log"
	f, err := os.Create(filename)
	if err != nil {
		t.Fatalf("could not create file. err: %s", err)
		return
	}
	defer os.Remove(filename)

	someday, err := time.Parse("2006-01-02", "2024-08-31")
	if err != nil {
		t.Fatalf("could not parse example date")
	}
	timestampsWritten := []int64{
		someday.Add(30 * time.Second).Unix(),
		someday.Add(60 * time.Second).Unix(),
		someday.Add(90 * time.Second).Unix(),
		someday.Add(120 * time.Second).Unix(),
	}

	_, err = f.WriteString(fmt.Sprintf(
		"%d\n%d\n%d\n%d",
		timestampsWritten[0],
		timestampsWritten[1],
		timestampsWritten[2],
		timestampsWritten[3]))
	if err != nil {
		t.Fatalf("could not write data to file")
	}

	timestampesRead := ReadTimestampsFromFile()
	for _, ts := range timestampesRead {
		if !slices.Contains(timestampsWritten, ts) {
			t.Errorf("expected timestamp %d not found in file", ts)
		}
	}
}

// benchmarkDuration is how long each BenchmarkServer variant sends requests.
const benchmarkDuration = 10 * time.Second

// BenchmarkServer runs the server the way main does (real clock, real file
// writes) and sends it real HTTP requests at a fixed rate for
// benchmarkDuration. Each request is sent on schedule whether or not earlier
// ones have been answered, like independent clients would. Requests still
// unanswered a second after sending stops are counted as unanswered.
// queued-writes is how many file writes were still waiting when sending
// stopped: a large number means the server is falling behind even if it is
// still answering quickly, because the queue hasn't filled up yet.
//
// Each variant runs once for a fixed time, so b.N and ns/op don't apply.
func BenchmarkServer(b *testing.B) {
	dir, err := os.MkdirTemp("", "request_counter_bench")
	if err != nil {
		b.Fatalf("could not create temp dir. err: %s", err)
	}
	defer os.RemoveAll(dir)
	FILE_NAME = filepath.Join(dir, "request_timestamps.log")

	for _, rps := range []int{10, 100, 1_000, 10_000} {
		b.Run(fmt.Sprintf("rps=%d", rps), func(b *testing.B) {
			// Start with a minute of history at this rate, as if the server had
			// already been running, so the window is full from the first request.
			history := make([]int64, 60*rps)
			minuteAgo := time.Now().Unix() - 60
			for i := range history {
				history[i] = minuteAgo + int64(i/rps)
			}

			rc := RequestCounter{
				requests:              history,
				latestCurrentRequests: make(chan []int64, 10_000),
				getNowTime:            time.Now,
			}
			go WaitForFileWrites(rc.latestCurrentRequests)
			server := httptest.NewServer(http.HandlerFunc(rc.HandleRequest))

			client := &http.Client{Transport: &http.Transport{
				MaxConnsPerHost:     1_000,
				MaxIdleConnsPerHost: 1_000,
			}}
			ctx, cancel := context.WithTimeout(context.Background(), benchmarkDuration+time.Second)
			defer cancel()

			var (
				wg        sync.WaitGroup
				mu        sync.Mutex
				latencies []time.Duration
			)
			sent := rps * int(benchmarkDuration/time.Second)
			interval := time.Second / time.Duration(rps)
			start := time.Now()
			for i := range sent {
				time.Sleep(time.Until(start.Add(time.Duration(i) * interval)))
				wg.Add(1)
				go func() {
					defer wg.Done()
					sentAt := time.Now()
					req, _ := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
					resp, err := client.Do(req)
					if err != nil {
						return
					}
					io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
					mu.Lock()
					latencies = append(latencies, time.Since(sentAt))
					mu.Unlock()
				}()
			}
			wg.Wait()
			queuedWrites := len(rc.latestCurrentRequests)

			// Throw away the queued writes so handlers stuck on a full queue can
			// finish, otherwise Close waits for the disk to catch up.
			go func() {
				for range rc.latestCurrentRequests {
				}
			}()
			server.Close()

			answered := len(latencies)
			b.ReportMetric(0, "ns/op")
			b.ReportMetric(float64(answered)/benchmarkDuration.Seconds(), "answered/s")
			b.ReportMetric(float64(sent-answered), "unanswered")
			b.ReportMetric(float64(queuedWrites), "queued-writes")
			if answered > 0 {
				slices.Sort(latencies)
				b.ReportMetric(latencies[answered/2].Seconds()*1000, "p50-ms")
				b.ReportMetric(latencies[answered*99/100].Seconds()*1000, "p99-ms")
			}
		})
	}
}
