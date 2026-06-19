/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

// Command controller runs the Iris control plane: a controller-runtime manager
// hosting the Relay reconcilers and the validating webhook.
package main

import (
	"context"
	"fmt"
	"os"
)

// Build metadata, injected at build time via -ldflags. sentryRelease is the
// unified philprime release identifier (see docs/distribution.md).
var (
	version       = "dev"
	commit        = "none"
	date          = "unknown"
	sentryRelease = ""
)

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "controller exited with error: %s\n", err)
		os.Exit(1)
	}
}
