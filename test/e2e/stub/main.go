/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

// Command stub is a tiny HTTP destination for the end-to-end suite. It records
// the bodies it receives so a test can assert a relayed message arrived, and it
// can be pointed at a failing path to exercise the required-destination retry
// contract. It is not part of the shipped product.
package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
)

type recorder struct {
	mu       sync.Mutex
	received []string
}

func (r *recorder) record(body string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.received = append(r.received, body)
}

func (r *recorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.received))
	copy(out, r.received)
	return out
}

func main() {
	addr := os.Getenv("STUB_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	rec := &recorder{}
	mux := http.NewServeMux()

	// /in records the request body and acknowledges, the happy path.
	mux.HandleFunc("/in", func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		rec.record(string(body))
		w.WriteHeader(http.StatusOK)
	})

	// /fail always errors so a required destination forces the relay to signal a
	// retry to Postfix.
	mux.HandleFunc("/fail", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "stub forced failure", http.StatusInternalServerError)
	})

	// /requests returns the recorded bodies as JSON for assertions.
	mux.HandleFunc("/requests", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(rec.snapshot())
	})

	log.Printf("e2e stub listening on %s", addr)
	server := &http.Server{Addr: addr, Handler: mux}
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("stub server: %v", err)
	}
}
