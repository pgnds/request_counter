package naive

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pgnds/request_counter/internal/counter"
)

type naiveCounter struct {
	mu         sync.Mutex
	file       string
	timestamps []int64
}

func New(file string) (counter.Counter, error) {
	data, err := os.ReadFile(file)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	c := &naiveCounter{file: file}
	for _, line := range strings.Fields(string(data)) {
		ts, err := strconv.ParseInt(line, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%s: bad timestamp %q", file, line)
		}
		c.timestamps = append(c.timestamps, ts)
	}

	return c, nil
}

func (c *naiveCounter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	now := time.Now().Unix()

	c.mu.Lock()
	defer c.mu.Unlock()

	c.timestamps = append(c.timestamps, now)

	// drop ones older than 1 minute
	for len(c.timestamps) > 0 && c.timestamps[0] < now-60 {
		c.timestamps = c.timestamps[1:]
	}

	var sb strings.Builder
	for _, ts := range c.timestamps {
		fmt.Fprintln(&sb, ts)
	}
	err := os.WriteFile(c.file, []byte(sb.String()), 0o644)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Fprint(w, len(c.timestamps))
}

func (c *naiveCounter) Close() error {
	return nil
}
