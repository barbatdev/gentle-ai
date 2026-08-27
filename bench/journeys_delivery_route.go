package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// deliveryRouteCapability is the read-only route-selection boundary added by
// #3154 WU7. These journeys prove only the built CLI's observed-origin routing;
// they do not imply provider actions, adapters, or remote mutation authority.
var deliveryRouteCapability = &Capability{
	// delivery-route does not render subcommand help with its flags, so probe the
	// exact command shape against the fixture repository instead of treating a
	// help-text omission as an unsupported built command.
	Probe: []string{"delivery-route", "--cwd", ".", "--json"},
}

type deliveryRouteEnvelope struct {
	Contract         string `json:"contract"`
	ObservedIdentity struct {
		OriginPresent bool   `json:"origin_present"`
		Host          string `json:"host"`
		URLCount      int    `json:"url_count"`
	} `json:"observed_identity"`
	Provider                string `json:"provider"`
	Status                  string `json:"status"`
	Route                   string `json:"route"`
	CapabilityEvidence      string `json:"capability_evidence"`
	AdapterCapability       string `json:"adapter_capability"`
	Reason                  string `json:"reason"`
	Continuation            string `json:"continuation"`
	RemoteMutationAuthority string `json:"remote_mutation_authority"`
}

func deliveryRouteRepoWithOrigin(origin string) func(*Sandbox) error {
	return func(sandbox *Sandbox) error {
		if err := baseRepo(sandbox); err != nil {
			return err
		}
		if err := sandbox.git(sandbox.Repo, "remote", "add", "origin", origin); err != nil {
			return err
		}
		return rememberDeliveryRouteLocalState(sandbox)
	}
}

// rememberDeliveryRouteLocalState records the local Git state this read-only
// command must preserve: origin configuration, refs, and worktree/index state.
func rememberDeliveryRouteLocalState(sandbox *Sandbox) error {
	state, err := deliveryRouteLocalState(sandbox)
	if err != nil {
		return err
	}
	sandbox.Scratch["delivery-route-local-state"] = state
	return nil
}

func requireDeliveryRouteLocalStateUnchanged(sandbox *Sandbox) error {
	before, found := sandbox.Scratch["delivery-route-local-state"]
	if !found {
		return fmt.Errorf("delivery-route fixture did not record local Git state")
	}
	after, err := deliveryRouteLocalState(sandbox)
	if err != nil {
		return err
	}
	if after != before {
		return fmt.Errorf("delivery-route changed local Git remote configuration, refs, or worktree state")
	}
	return nil
}

func deliveryRouteLocalState(sandbox *Sandbox) (string, error) {
	config, err := os.ReadFile(sandbox.Repo + "/.git/config")
	if err != nil {
		return "", fmt.Errorf("read local Git config: %w", err)
	}
	refs, err := deliveryRouteGitOutput(sandbox, "show-ref", "--head")
	if err != nil {
		return "", err
	}
	status, err := deliveryRouteGitOutput(sandbox, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return "", err
	}
	return string(config) + "\x00" + refs + "\x00" + status, nil
}

func deliveryRouteGitOutput(sandbox *Sandbox, args ...string) (string, error) {
	command := exec.Command("git", append([]string{"-C", sandbox.Repo}, args...)...)
	command.Env = sandbox.env()
	for index, value := range command.Env {
		if strings.HasPrefix(value, "GIT_TRACE=") {
			command.Env[index] = "GIT_TRACE="
		}
	}
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func deliveryRouteArgsForSandbox(sandbox *Sandbox) ([]string, error) {
	return []string{"delivery-route", "--cwd", sandbox.Repo, "--json"}, nil
}

func requireGitHubDeliveryRouteBlocked(sandbox *Sandbox, observation Observation) error {
	if err := requireDeliveryRouteLocalStateUnchanged(sandbox); err != nil {
		return err
	}
	if observation.ExitCode == 0 {
		return fmt.Errorf("GitHub route selection succeeded without trusted capability evidence")
	}
	envelope, err := parseDeliveryRouteEnvelope(observation)
	if err != nil {
		return err
	}
	if err := requireDeliveryRouteIdentity(envelope, "github.com"); err != nil {
		return err
	}
	if envelope.Status != "blocked" || envelope.Reason != "github_capability_unproven" {
		return fmt.Errorf("GitHub route = status %q route %q reason %q, want blocked with github_capability_unproven", envelope.Status, envelope.Route, envelope.Reason)
	}
	if envelope.Route == "portable_policy" {
		return fmt.Errorf("GitHub route fell back to portable policy")
	}
	if envelope.Route != "github" {
		return fmt.Errorf("GitHub route = %q, want github rather than an unproven fallback", envelope.Route)
	}
	if envelope.Continuation != "obtain trusted WU4 capability evidence before GitHub delivery" {
		return fmt.Errorf("GitHub route continuation = %q, want actionable WU4 evidence recovery", envelope.Continuation)
	}
	if strings.Contains(envelope.Continuation, "portable") {
		return fmt.Errorf("GitHub route fell back to portable policy: %q", envelope.Continuation)
	}
	return requireDeliveryRouteLimitations(envelope)
}

func requirePortableDeliveryRoute(sandbox *Sandbox, observation Observation) error {
	if err := requireDeliveryRouteLocalStateUnchanged(sandbox); err != nil {
		return err
	}
	if observation.ExitCode != 0 {
		return fmt.Errorf("portable route selection exited %d: %s", observation.ExitCode, firstLine(observation.Stderr))
	}
	envelope, err := parseDeliveryRouteEnvelope(observation)
	if err != nil {
		return err
	}
	if err := requireDeliveryRouteIdentity(envelope, "gitlab.com"); err != nil {
		return err
	}
	if envelope.Status != "ready" || envelope.Route != "portable_policy" || envelope.Reason != "portable_policy" {
		return fmt.Errorf("GitLab route = status %q route %q reason %q, want ready/portable_policy/portable_policy", envelope.Status, envelope.Route, envelope.Reason)
	}
	if envelope.Continuation != "follow the portable delivery policy; adapter capability and remote mutation authority are not granted" {
		return fmt.Errorf("GitLab route continuation = %q, want portable-policy limitation", envelope.Continuation)
	}
	return requireDeliveryRouteLimitations(envelope)
}

func parseDeliveryRouteEnvelope(observation Observation) (deliveryRouteEnvelope, error) {
	var envelope deliveryRouteEnvelope
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &envelope); err != nil {
		return envelope, fmt.Errorf("parse delivery-route JSON: %w (stderr: %s)", err, firstLine(observation.Stderr))
	}
	if envelope.Contract != "gentle-ai.delivery-route/v1" {
		return envelope, fmt.Errorf("delivery-route contract = %q, want gentle-ai.delivery-route/v1", envelope.Contract)
	}
	return envelope, nil
}

func requireDeliveryRouteIdentity(envelope deliveryRouteEnvelope, provider string) error {
	if !envelope.ObservedIdentity.OriginPresent || envelope.ObservedIdentity.Host != provider || envelope.ObservedIdentity.URLCount != 1 || envelope.Provider != provider {
		return fmt.Errorf("delivery-route identity = %+v provider %q, want one observed %s origin", envelope.ObservedIdentity, envelope.Provider, provider)
	}
	return nil
}

func requireDeliveryRouteLimitations(envelope deliveryRouteEnvelope) error {
	if envelope.CapabilityEvidence != "unproven" || envelope.AdapterCapability != "unproven" || envelope.RemoteMutationAuthority != "not_granted" {
		return fmt.Errorf("delivery-route limitations = capability %q adapter %q remote authority %q, want unproven/unproven/not_granted", envelope.CapabilityEvidence, envelope.AdapterCapability, envelope.RemoteMutationAuthority)
	}
	return nil
}

func deliveryRouteJourneys() []Journey {
	return []Journey{
		{
			ID:     "j119-github-route-requires-trusted-capability-evidence",
			Review: reviewUntouched,
			Title:  "#3154 WU8: observed GitHub origin blocks without trusted capability evidence",
			Source: "#3154 WU8: built CLI route selection is read-only and does not grant remote mutation authority",
			Steps: []Step{
				{Name: "fixture: repository with observed GitHub origin", Fixture: deliveryRouteRepoWithOrigin("https://github.com/gentle-ai/demo.git")},
				{Name: "GitHub route blocks without portable fallback and preserves local Git state", Requires: deliveryRouteCapability, Args: deliveryRouteArgsForSandbox, After: requireGitHubDeliveryRouteBlocked},
			},
		},
		{
			ID:     "j120-gitlab-route-selects-portable-policy-without-authority",
			Review: reviewUntouched,
			Title:  "#3154 WU8: observed GitLab origin selects portable policy without remote authority",
			Source: "#3154 WU8: built CLI route selection is read-only and does not imply adapter or remote mutation capability",
			Steps: []Step{
				{Name: "fixture: repository with observed GitLab origin", Fixture: deliveryRouteRepoWithOrigin("https://gitlab.com/gentle-ai/demo.git")},
				{Name: "GitLab route selects portable policy and preserves local Git state", Requires: deliveryRouteCapability, Args: deliveryRouteArgsForSandbox, After: requirePortableDeliveryRoute},
			},
		},
	}
}
