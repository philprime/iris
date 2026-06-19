/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

package controller

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

// testEnvCfg is the rest.Config for the envtest API server, or nil when envtest
// binaries are unavailable (KUBEBUILDER_ASSETS unset). The envtest-backed tests
// skip themselves in that case, so the fake-client unit tests still run with a
// plain `go test`. The `make test` target always provides the assets.
var testEnvCfg *rest.Config

func TestMain(m *testing.M) {
	os.Exit(runTestMain(m))
}

func runTestMain(m *testing.M) int {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		return m.Run()
	}

	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err := testEnv.Start()
	if err != nil {
		fmt.Fprintf(os.Stderr, "start envtest: %s\n", err)
		return 1
	}
	defer func() { _ = testEnv.Stop() }()

	testEnvCfg = cfg
	return m.Run()
}

// envtestClient builds a client against the envtest API server, skipping the
// test when envtest is unavailable.
func envtestClient(t *testing.T) client.Client {
	t.Helper()
	if testEnvCfg == nil {
		t.Skip("envtest unavailable: KUBEBUILDER_ASSETS not set (run via `make test`)")
	}
	c, err := client.New(testEnvCfg, client.Options{Scheme: testScheme(t)})
	if err != nil {
		t.Fatalf("build envtest client: %v", err)
	}
	return c
}
