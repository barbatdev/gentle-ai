package deliveryroute

import (
	"context"
	"errors"
	"testing"
)

func TestOriginHostRecognizesHTTPSAndSSHForms(t *testing.T) {
	for raw, want := range map[string]string{
		"https://gitlab.com/a/r.git": "gitlab.com", "ssh://git@bitbucket.org/a/r.git": "bitbucket.org", "git@github.com:a/r.git": "github.com",
	} {
		got, ok := originHost(raw)
		if !ok || got != want {
			t.Fatalf("originHost(%q) = %q, %t", raw, got, ok)
		}
	}
	if _, ok := originHost("https://"); ok {
		t.Fatal("malformed URL accepted")
	}
}

func TestResolveReportsUnreadableRepository(t *testing.T) {
	got := Resolve(context.Background(), t.TempDir())
	if got.Status != StatusBlocked || got.Reason != ReasonRepositoryUnavailable {
		t.Fatalf("Resolve() = %#v", got)
	}
}

func TestResolveFailsClosedFromObservedOriginIdentity(t *testing.T) {
	tests := []struct {
		name   string
		urls   []string
		err    error
		want   Status
		reason Reason
		route  Route
	}{
		{"missing origin", nil, nil, StatusBlocked, ReasonOriginMissing, RouteNone},
		{"malformed URL", []string{"not a URL"}, nil, StatusBlocked, ReasonOriginMalformed, RouteNone},
		{"conflicting URLs", []string{"https://gitlab.com/a/r.git", "git@github.com:a/r.git"}, nil, StatusBlocked, ReasonOriginAmbiguous, RouteNone},
		{"GitHub unproven", []string{"git@github.com:a/r.git"}, nil, StatusBlocked, ReasonGitHubCapabilityUnproven, RouteGitHub},
		{"portable host", []string{"https://gitlab.com/a/r.git"}, nil, StatusReady, ReasonPortablePolicy, RoutePortablePolicy},
		{"unknown host", []string{"ssh://git@forge.example/a/r.git"}, nil, StatusBlocked, ReasonProviderUnknown, RouteNone},
		{"same identity duplicates", []string{"https://gitlab.com/a/r.git", "git@gitlab.com:a/r.git"}, nil, StatusReady, ReasonPortablePolicy, RoutePortablePolicy},
		{"repository error", nil, errors.New("not a repository"), StatusBlocked, ReasonRepositoryUnavailable, RouteNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := originURLs
			originURLs = func(context.Context, string) ([]string, error) { return tt.urls, tt.err }
			t.Cleanup(func() { originURLs = original })
			got := Resolve(context.Background(), t.TempDir())
			if got.Status != tt.want || got.Reason != tt.reason || got.Route != tt.route {
				t.Fatalf("Resolve() = %#v", got)
			}
			if got.RemoteMutationAuthority != "not_granted" || got.AdapterCapability != "unproven" {
				t.Fatalf("authority/capability claim = %#v", got)
			}
		})
	}
}
