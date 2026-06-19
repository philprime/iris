/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

// Command relay runs the stateless data-plane transformer: a go-smtp server
// that filters, transforms, and fans inbound mail out to the configured
// destinations.
package main

import (
	"context"
	"fmt"
	"os"
)

// Build metadata, injected at build time via -ldflags.
var (
	version       = "dev"
	commit        = "none"
	date          = "unknown"
	sentryRelease = ""
)

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "relay exited with error: %s\n", err)
		os.Exit(1)
	}
}
