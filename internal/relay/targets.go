/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

package relay

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/philprime/iris/api/v1alpha1"
)

// Mount subdirectories under the relay's config mount. The controller mounts a
// destination's referenced Secret and Jsonnet ConfigMap here, since the relay
// pod has no Kubernetes API access.
const (
	secretsDir    = "secrets"
	transformsDir = "transforms"
)

// BuildTargets resolves a relay Config into deliverable targets, reading each
// destination's auth token and Jsonnet program from the files the controller
// mounts under dir. The client is shared by all HTTP targets.
func BuildTargets(cfg Config, dir string, client *http.Client) ([]Target, error) {
	targets := make([]Target, 0, len(cfg.Destinations))
	for _, dest := range cfg.Destinations {
		target := Target{Name: dest.Name, Required: required(dest.Required)}

		switch {
		case dest.HTTP != nil:
			http, err := httpTargetFromSpec(dest.HTTP, dir, client)
			if err != nil {
				return nil, fmt.Errorf("destination %q: %w", dest.Name, err)
			}
			target.HTTP = http
		case dest.SMTP != nil:
			smtp, err := smtpTargetFromSpec(dest.SMTP, dir)
			if err != nil {
				return nil, fmt.Errorf("destination %q: %w", dest.Name, err)
			}
			target.SMTP = smtp
		default:
			return nil, fmt.Errorf("destination %q has no delivery method", dest.Name)
		}

		targets = append(targets, target)
	}
	return targets, nil
}

func httpTargetFromSpec(spec *v1alpha1.HTTPDestination, dir string, client *http.Client) (*HTTPTarget, error) {
	target := &HTTPTarget{
		URL:    spec.URL,
		Method: spec.Method,
		Format: spec.PayloadFormat,
		Client: client,
	}
	if spec.AuthSecretRef != nil {
		token, err := readSecret(dir, spec.AuthSecretRef)
		if err != nil {
			return nil, err
		}
		target.AuthToken = token
	}
	if spec.Transform != nil {
		program, err := os.ReadFile(transformPath(dir, spec.Transform.JsonnetConfigMapRef))
		if err != nil {
			return nil, fmt.Errorf("read jsonnet transform: %w", err)
		}
		target.Jsonnet = string(program)
	}
	return target, nil
}

func smtpTargetFromSpec(spec *v1alpha1.SMTPDestination, dir string) (*SMTPTarget, error) {
	target := &SMTPTarget{
		Host:     spec.Host,
		Port:     int(spec.Port),
		StartTLS: spec.StartTLS,
	}
	if spec.AuthSecretRef != nil {
		credential, err := readSecret(dir, spec.AuthSecretRef)
		if err != nil {
			return nil, err
		}
		// A "user:password" credential carries both, otherwise it is a password.
		if user, password, ok := strings.Cut(credential, ":"); ok {
			target.Username, target.Password = user, password
		} else {
			target.Password = credential
		}
	}
	return target, nil
}

func readSecret(dir string, ref *v1alpha1.SecretKeyRef) (string, error) {
	value, err := os.ReadFile(filepath.Join(dir, secretsDir, ref.Name, ref.Key))
	if err != nil {
		return "", fmt.Errorf("read secret %s/%s: %w", ref.Name, ref.Key, err)
	}
	return strings.TrimSpace(string(value)), nil
}

func transformPath(dir string, ref v1alpha1.ConfigMapKeyRef) string {
	return filepath.Join(dir, transformsDir, ref.Name, ref.Key)
}

// required resolves a destination's optional required flag, defaulting to true.
func required(flag *bool) bool {
	return flag == nil || *flag
}
