//go:build e2e

/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

// Package e2e drives the whole stack in a kind cluster: it deploys a stub HTTP
// destination and two Relays, sends SMTP through the Postfix ingress, and
// asserts a message reaches the stub and that a required-destination failure
// makes the relay signal a retry. The cluster, images, and chart are provisioned
// by `make test-e2e` before this suite runs.
package e2e

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlcfg "sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/philprime/iris/api/v1alpha1"
)

// Fixture coordinates. They default to what `make deploy` (release "iris")
// produces and are overridable by the Makefile.
var (
	namespace      = envOr("E2E_NAMESPACE", "iris-system")
	postfixService = envOr("E2E_POSTFIX_SERVICE", "iris-iris-postfix")
	stubImage      = envOr("E2E_STUB_IMAGE", "ghcr.io/philprime/iris-e2e-stub:dev")
)

const (
	stubName    = "e2e-stub"
	happyRelay  = "e2e-happy"
	retryRelay  = "e2e-retry"
	happyDomain = "happy.example.com"
	retryDomain = "retry.example.com"
	// smtpNodePort is the pinned Postfix SMTP node port. The Makefile deploys the
	// chart with exposure.service.type=NodePort and this value, and the e2e kind
	// config (test/e2e/kind.yaml) maps it to the same host port so the suite can
	// reach the ingress through a real node port.
	smtpNodePort = 30025
)

var k8s client.Client

func TestMain(m *testing.M) {
	os.Exit(runMain(m))
}

func runMain(m *testing.M) int {
	cfg, err := ctrlcfg.GetConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "no kubeconfig (run via `make test-e2e`): %s\n", err)
		return 1
	}
	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{corev1.AddToScheme, appsv1.AddToScheme, v1alpha1.AddToScheme} {
		if err := add(scheme); err != nil {
			fmt.Fprintf(os.Stderr, "build scheme: %s\n", err)
			return 1
		}
	}
	k8s, err = client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		fmt.Fprintf(os.Stderr, "build client: %s\n", err)
		return 1
	}

	ctx := context.Background()
	if err := applyFixtures(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "apply fixtures: %s\n", err)
		return 1
	}
	if err := waitReady(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "wait for fixtures: %s\n", err)
		return 1
	}
	return m.Run()
}

// applyFixtures creates the stub destination and the two Relays.
func applyFixtures(ctx context.Context) error {
	labels := map[string]string{"app": stubName}
	stubURL := func(path string) string {
		return fmt.Sprintf("http://%s.%s.svc.cluster.local:8080%s", stubName, namespace, path)
	}

	objs := []client.Object{
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: stubName, Namespace: namespace, Labels: labels},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{MatchLabels: labels},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: labels},
					Spec: corev1.PodSpec{Containers: []corev1.Container{{
						Name:            "stub",
						Image:           stubImage,
						ImagePullPolicy: corev1.PullIfNotPresent,
						Ports:           []corev1.ContainerPort{{ContainerPort: 8080}},
					}}},
				},
			},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: stubName, Namespace: namespace, Labels: labels},
			Spec: corev1.ServiceSpec{
				Selector: labels,
				Ports:    []corev1.ServicePort{{Port: 8080, TargetPort: intstr.FromInt32(8080)}},
			},
		},
		relayTo(happyRelay, happyDomain, stubURL("/in")),
		relayTo(retryRelay, retryDomain, stubURL("/fail")),
	}

	for _, obj := range objs {
		if err := k8s.Create(ctx, obj); err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("create %T %s: %w", obj, obj.GetName(), err)
		}
	}
	return nil
}

// relayTo builds a Relay routing a domain to a single required HTTP destination
// that posts the raw message.
func relayTo(name, domain, url string) *v1alpha1.Relay {
	return &v1alpha1.Relay{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: v1alpha1.RelaySpec{
			Routes: []v1alpha1.Route{{Domain: domain}},
			Destinations: []v1alpha1.Destination{{
				Name: "stub",
				HTTP: &v1alpha1.HTTPDestination{URL: url, PayloadFormat: v1alpha1.PayloadFormatRaw},
			}},
		},
	}
}

// waitReady blocks until the stub and both relay Deployments are available.
func waitReady(ctx context.Context) error {
	deployments := []string{stubName, "relay-" + happyRelay, "relay-" + retryRelay}
	deadline := time.Now().Add(3 * time.Minute)
	for _, name := range deployments {
		if err := waitDeployment(ctx, name, deadline); err != nil {
			return err
		}
	}
	return nil
}

func waitDeployment(ctx context.Context, name string, deadline time.Time) error {
	for {
		var dep appsv1.Deployment
		err := k8s.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &dep)
		if err == nil && dep.Status.AvailableReplicas > 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("deployment %s not available in time (lastErr=%v)", name, err)
		}
		time.Sleep(2 * time.Second)
	}
}

// Feature: end-to-end delivery
// Scenario: a message sent to the Postfix ingress reaches the relay's HTTP destination
func TestDeliveryThroughPostfix(t *testing.T) {
	stubStop := portForward(t, "svc/"+stubName, 18080, 8080)
	defer stubStop()

	marker := "iris-e2e-" + fmt.Sprint(time.Now().UnixNano())

	// Retry the send: Postfix may still be reloading the freshly rendered routing
	// maps when the suite starts, so a recipient can briefly be rejected. A fresh
	// port-forward per attempt is resilient to kubectl port-forward dropping.
	deadline := time.Now().Add(2 * time.Minute)
	var lastOut string
	for {
		lastOut = sendThroughPostfix(t, "user@"+happyDomain, marker)
		if receivedMarker(t, "http://localhost:18080/requests", marker) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("message with marker %q never reached the stub; last attempt:\n%s", marker, lastOut)
		}
		time.Sleep(3 * time.Second)
	}
}

// sendThroughPostfix opens a short-lived port-forward to the Postfix ingress and
// injects one message, returning a combined diagnostic of the forward and swaks
// output. It never fails the test so the caller can retry.
func sendThroughPostfix(t *testing.T, to, marker string) string {
	t.Helper()
	cmd := exec.Command("kubectl", "port-forward", "-n", namespace, "svc/"+postfixService, "11025:25")
	var pfErr strings.Builder
	cmd.Stdout = io.Discard
	cmd.Stderr = &pfErr
	if err := cmd.Start(); err != nil {
		return fmt.Sprintf("start port-forward: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()
	if !waitListening("localhost:11025", 15*time.Second) {
		return "port-forward never started listening:\n" + pfErr.String()
	}

	out, err := sendMail("localhost:11025", "sender@external.test", to, marker)
	if err != nil {
		return fmt.Sprintf("swaks error: %v\n%s\nport-forward log:\n%s", err, out, pfErr.String())
	}
	return out
}

// Feature: NodePort exposure
// Scenario: the chart publishes Postfix through a NodePort Service with the pinned node ports
func TestPostfixServiceIsNodePort(t *testing.T) {
	var svc corev1.Service
	if err := k8s.Get(context.Background(), client.ObjectKey{Namespace: namespace, Name: postfixService}, &svc); err != nil {
		t.Fatalf("get postfix service %q: %v", postfixService, err)
	}
	if svc.Spec.Type != corev1.ServiceTypeNodePort {
		t.Fatalf("expected Service type NodePort, got %q", svc.Spec.Type)
	}
	want := map[string]int32{"smtp": 30025, "submission": 30587, "smtps": 30465}
	for _, p := range svc.Spec.Ports {
		expected, ok := want[p.Name]
		if !ok {
			continue
		}
		if p.NodePort != expected {
			t.Errorf("port %q: expected nodePort %d, got %d", p.Name, expected, p.NodePort)
		}
		delete(want, p.Name)
	}
	if len(want) != 0 {
		t.Fatalf("Service is missing expected named ports with pinned node ports: %v", want)
	}
}

// Feature: NodePort exposure
// Scenario: the Postfix SMTP listener is reachable through the mapped node port
func TestSMTPReachableViaNodePort(t *testing.T) {
	addr := fmt.Sprintf("localhost:%d", smtpNodePort)
	// The kind config maps the node port to the same host port. Retry briefly:
	// Postfix may still be starting when the suite begins.
	deadline := time.Now().Add(60 * time.Second)
	for {
		banner, err := readSMTPBanner(addr, 5*time.Second)
		if err == nil && strings.HasPrefix(banner, "220") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("no SMTP 220 banner on node port %s (lastErr=%v, banner=%q)", addr, err, banner)
		}
		time.Sleep(2 * time.Second)
	}
}

// readSMTPBanner dials addr, reads the SMTP greeting line, and politely quits.
func readSMTPBanner(addr string, timeout time.Duration) (string, error) {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return "", err
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return line, err
	}
	_, _ = conn.Write([]byte("QUIT\r\n"))
	return strings.TrimSpace(line), nil
}

// Feature: delivery contract
// Scenario: a failure to a required destination makes the relay signal a retry (SMTP 4xx)
func TestRequiredDestinationFailureSignalsRetry(t *testing.T) {
	// Send straight to the relay so the synchronous fan-out result is visible in
	// the SMTP reply, rather than waiting on Postfix's async requeue.
	stop := portForward(t, "svc/relay-"+retryRelay, 12025, 25)
	defer stop()

	marker := "iris-e2e-retry-" + fmt.Sprint(time.Now().UnixNano())
	out, err := sendMail("localhost:12025", "sender@external.test", "user@"+retryDomain, marker)
	if err == nil {
		t.Fatalf("expected a 4xx retry signal, but the send succeeded:\n%s", out)
	}
	if !strings.Contains(out, "451") {
		t.Fatalf("expected SMTP 451 retry signal from the relay, got:\n%s", out)
	}
}

// sendMail injects a message with swaks and returns its combined output. A
// non-nil error means swaks saw a non-2xx SMTP reply.
func sendMail(server, from, to, marker string) (string, error) {
	cmd := exec.Command("swaks",
		"--server", server,
		"--helo", "iris-e2e.test",
		"--from", from,
		"--to", to,
		"--h-Subject", marker,
		"--body", "e2e test body "+marker,
		"--timeout", "20s",
	)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// receivedMarker reports whether the stub recorded a request body containing the
// marker.
func receivedMarker(t *testing.T, url, marker string) bool {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	return strings.Contains(string(body), marker)
}

// portForward runs `kubectl port-forward` in the background and waits until the
// local port accepts connections, returning a stop function.
func portForward(t *testing.T, target string, local, remote int) func() {
	t.Helper()
	cmd := exec.Command("kubectl", "port-forward", "-n", namespace, target,
		fmt.Sprintf("%d:%d", local, remote))
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatalf("start port-forward %s: %v", target, err)
	}
	stop := func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}
	if !waitListening(fmt.Sprintf("localhost:%d", local), 30*time.Second) {
		stop()
		t.Fatalf("port-forward to %s never became ready", target)
	}
	return stop
}

// waitListening reports whether the address accepts a TCP connection within the
// timeout.
func waitListening(addr string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			_ = conn.Close()
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
