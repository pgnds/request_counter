package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"maps"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/pgnds/request_counter/internal/counter"
	"github.com/pgnds/request_counter/internal/naive"
)

var counters = map[string]func(file string) (counter.Counter, error){
	"naive": naive.New,
}

func main() {
	names := strings.Join(slices.Sorted(maps.Keys(counters)), ", ")

	name := flag.String("counter", "naive", "counter implementation:"+names)
	flag.Parse()

	newCounter, ok := counters[*name]
	if !ok {
		log.Fatalf("unknown counter %q, choose one of: %s", *name, names)
	}

	counter, err := newCounter(*name + ".log")
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	server := &http.Server{Addr: ":80", Handler: counter}
	go func() {
		err := server.ListenAndServe()
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	// wait for Ctrl-C signal
	<-ctx.Done()

	// stop taking requests, let in-flight finish (5 seconds deadline)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = server.Shutdown(shutdownCtx)
	if err != nil {
		log.Print(err)
	}

	// save counters state before exit
	err = counter.Close()
	if err != nil {
		log.Fatal(err)
	}
}
