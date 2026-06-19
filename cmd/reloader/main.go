/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

// Command reloader watches the mounted Postfix routing maps and runs postfix
// reload when they change, so the daemons re-read the texthash maps. It is
// baked into the Postfix image as the entrypoint companion process.
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
		fmt.Fprintf(os.Stderr, "reloader exited with error: %s\n", err)
		os.Exit(1)
	}
}
