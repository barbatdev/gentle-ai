// Package deliveryroute resolves the read-only delivery route implied by origin.
package deliveryroute

import (
	"context"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)

const Contract = "gentle-ai.delivery-route/v1"

var ErrBlocked = errors.New("delivery route blocked")

type Status string
type Route string
type Reason string

const (
	StatusReady                    Status = "ready"
	StatusBlocked                  Status = "blocked"
	RouteNone                      Route  = "none"
	RouteGitHub                    Route  = "github"
	RoutePortablePolicy            Route  = "portable_policy"
	ReasonOriginMissing            Reason = "origin_missing"
	ReasonOriginMalformed          Reason = "origin_malformed"
	ReasonOriginAmbiguous          Reason = "origin_ambiguous"
	ReasonRepositoryUnavailable    Reason = "repository_unavailable"
	ReasonProviderUnknown          Reason = "provider_unknown"
	ReasonGitHubCapabilityUnproven Reason = "github_capability_unproven"
	ReasonPortablePolicy           Reason = "portable_policy"
)

type Identity struct {
	OriginPresent bool   `json:"origin_present"`
	Host          string `json:"host,omitempty"`
	URLCount      int    `json:"url_count"`
}

type Result struct {
	Contract                string   `json:"contract"`
	ObservedIdentity        Identity `json:"observed_identity"`
	Provider                string   `json:"provider"`
	Status                  Status   `json:"status"`
	Route                   Route    `json:"route"`
	CapabilityEvidence      string   `json:"capability_evidence"`
	AdapterCapability       string   `json:"adapter_capability"`
	Reason                  Reason   `json:"reason"`
	Continuation            string   `json:"continuation"`
	RemoteMutationAuthority string   `json:"remote_mutation_authority"`
}

// githubCapabilityEvidence is deliberately package-owned: future WU4 evidence may
// enter only through this typed seam, never through CLI flags or repository files.
type githubCapabilityEvidence struct{ trusted bool }

var originURLs = readOriginURLs

func Resolve(ctx context.Context, cwd string) Result {
	return resolve(ctx, cwd, githubCapabilityEvidence{})
}

func resolve(ctx context.Context, cwd string, github githubCapabilityEvidence) Result {
	result := blocked(ReasonRepositoryUnavailable, "use a readable Git repository with an origin remote")
	urls, err := originURLs(ctx, cwd)
	if err != nil {
		return result
	}
	result.ObservedIdentity.OriginPresent, result.ObservedIdentity.URLCount = len(urls) > 0, len(urls)
	if len(urls) == 0 {
		return block(result, ReasonOriginMissing, "configure one unambiguous origin remote")
	}
	host := ""
	for _, raw := range urls {
		candidate, ok := originHost(raw)
		if !ok {
			return block(result, ReasonOriginMalformed, "configure origin with a supported URL host identity")
		}
		if host != "" && host != candidate {
			return block(result, ReasonOriginAmbiguous, "configure origin URLs for one provider host")
		}
		host = candidate
	}
	result.ObservedIdentity.Host = host
	result.Provider = host
	switch host {
	case "github.com":
		result.Route = RouteGitHub
		if github.trusted {
			result.Status, result.Reason = StatusReady, "github_capability_verified"
			result.Continuation = "use the trusted GitHub delivery capability"
			return result
		}
		result.Reason, result.Continuation = ReasonGitHubCapabilityUnproven, "obtain trusted WU4 capability evidence before GitHub delivery"
	case "gitlab.com", "bitbucket.org":
		result.Status, result.Route, result.Reason = StatusReady, RoutePortablePolicy, ReasonPortablePolicy
		result.Continuation = "follow the portable delivery policy; adapter capability and remote mutation authority are not granted"
	default:
		result.Route, result.Reason, result.Continuation = RouteNone, ReasonProviderUnknown, "use an origin hosted by a recognized provider"
	}
	return result
}

func blocked(reason Reason, continuation string) Result {
	return block(Result{Contract: Contract, CapabilityEvidence: "unproven", AdapterCapability: "unproven", RemoteMutationAuthority: "not_granted"}, reason, continuation)
}

func block(result Result, reason Reason, continuation string) Result {
	result.Status, result.Route, result.Reason, result.Continuation = StatusBlocked, RouteNone, reason, continuation
	return result
}

func originHost(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if strings.Contains(raw, " ") {
		return "", false
	}
	if colon := strings.IndexByte(raw, ':'); colon > 0 && !strings.HasPrefix(raw[colon+1:], "//") && !strings.Contains(raw[:colon], "/") && !strings.Contains(raw[:colon], ":") {
		part := raw[:colon]
		if at := strings.LastIndexByte(part, '@'); at >= 0 {
			part = part[at+1:]
		}
		if part != "" && raw[colon+1:] != "" {
			return strings.ToLower(strings.TrimSuffix(part, ".")), true
		}
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Hostname() == "" {
		return "", false
	}
	return strings.ToLower(strings.TrimSuffix(parsed.Hostname(), ".")), true
}

func readOriginURLs(ctx context.Context, cwd string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "git", "-c", "core.hooksPath=/dev/null", "-C", cwd, "remote", "get-url", "--all", "origin")
	command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0")
	output, err := command.Output()
	if err != nil {
		return nil, err
	}
	return strings.Fields(string(output)), nil
}
