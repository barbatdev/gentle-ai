package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/deliveryroute"
)

func TestRunDeliveryRouteEmitsStableJSONAndBlockedExit(t *testing.T) {
	original := deliveryRouteResolve
	deliveryRouteResolve = func(context.Context, string) deliveryroute.Result {
		return deliveryroute.Result{Contract: deliveryroute.Contract, Status: deliveryroute.StatusBlocked, Route: deliveryroute.RouteGitHub, Reason: deliveryroute.ReasonGitHubCapabilityUnproven, Continuation: "obtain trusted WU4 capability evidence", AdapterCapability: "unproven", RemoteMutationAuthority: "not_granted"}
	}
	t.Cleanup(func() { deliveryRouteResolve = original })
	var out bytes.Buffer
	err := RunDeliveryRoute([]string{"--cwd", t.TempDir(), "--json"}, &out)
	if !errors.Is(err, deliveryroute.ErrBlocked) {
		t.Fatalf("error = %v, want blocked", err)
	}
	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("JSON = %q: %v", out.String(), err)
	}
	for key, want := range map[string]string{"contract": deliveryroute.Contract, "status": "blocked", "route": "github", "reason": "github_capability_unproven", "remote_mutation_authority": "not_granted"} {
		if got[key] != want {
			t.Fatalf("%s = %v, want %q", key, got[key], want)
		}
	}
}

func TestRunDeliveryRouteRequiresJSONAndCWD(t *testing.T) {
	if err := RunDeliveryRoute([]string{"--json"}, &bytes.Buffer{}); err == nil {
		t.Fatal("missing cwd accepted")
	}
	if err := RunDeliveryRoute([]string{"--cwd", t.TempDir()}, &bytes.Buffer{}); err == nil {
		t.Fatal("missing json accepted")
	}
}

func TestRunDeliveryRouteInvalidArgumentsNameUsageContinuation(t *testing.T) {
	const usage = "gentle-ai delivery-route --cwd <repo> --json"
	for _, testCase := range []struct {
		name string
		args []string
	}{
		{name: "missing cwd value", args: []string{"--cwd", "--json"}},
		{name: "unknown argument", args: []string{"--unknown"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			err := RunDeliveryRoute(testCase.args, &bytes.Buffer{})
			if err == nil {
				t.Fatal("RunDeliveryRoute() error = nil")
			}
			if !strings.Contains(err.Error(), usage) {
				t.Fatalf("RunDeliveryRoute() error = %q, want actionable usage %q", err, usage)
			}
		})
	}
}
