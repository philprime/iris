/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

package relay

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/go-jsonnet"

	"github.com/philprime/iris/api/v1alpha1"
)

// Render produces a destination's payload from the canonical envelope. The raw
// format forwards the original message bytes. The json format emits the
// canonical envelope, optionally remapped by a Jsonnet program that reads the
// envelope from the "envelope" external variable. It returns the body and its
// content type.
func Render(env *Envelope, raw []byte, format v1alpha1.PayloadFormat, jsonnetProgram string) ([]byte, string, error) {
	if format == v1alpha1.PayloadFormatRaw {
		return raw, "message/rfc822", nil
	}

	canonical, err := json.Marshal(env)
	if err != nil {
		return nil, "", fmt.Errorf("marshal canonical envelope: %w", err)
	}

	if strings.TrimSpace(jsonnetProgram) == "" {
		return canonical, "application/json", nil
	}

	vm := jsonnet.MakeVM()
	vm.ExtCode("envelope", string(canonical))
	out, err := vm.EvaluateAnonymousSnippet("transform.jsonnet", jsonnetProgram)
	if err != nil {
		return nil, "", fmt.Errorf("evaluate jsonnet transform: %w", err)
	}
	return []byte(out), "application/json", nil
}
