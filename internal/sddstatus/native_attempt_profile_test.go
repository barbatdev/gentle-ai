package sddstatus

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeAttemptProfile(t *testing.T) {
	for _, tt := range []struct {
		name    string
		path    string
		payload func(string) string
		prepare func(*testing.T, string)
		mutate  func(*testing.T, string)
		wantErr bool
	}{
		{"valid", "profile.json", nativeAttemptProfileJSON, nil, nil, false},
		{"unknown member", "profile.json", func(root string) string {
			return strings.TrimSuffix(nativeAttemptProfileJSON(root), "}") + `,"extra":"x"}`
		}, nil, nil, true},
		{"duplicate member", "profile.json", func(root string) string {
			return strings.TrimSuffix(nativeAttemptProfileJSON(root), "}") + `,"schema":"gentle-ai.native-attempt-profile/v1"}`
		}, nil, nil, true},
		{"unsupported schema", "profile.json", func(root string) string { return strings.Replace(nativeAttemptProfileJSON(root), "/v1\"", "/v2\"", 1) }, nil, nil, true},
		{"mistyped member", "profile.json", func(root string) string {
			return strings.Replace(nativeAttemptProfileJSON(root), `"root":`+fmt.Sprintf("%q", root), `"root":1`, 1)
		}, nil, nil, true},
		{"trailing input", "profile.json", func(root string) string { return nativeAttemptProfileJSON(root) + "{}" }, nil, nil, true},
		{"bounded", "profile.json", func(root string) string {
			return nativeAttemptProfileJSON(root) + strings.Repeat(" ", nativeAttemptProfileMaximumBytes)
		}, nil, nil, true},
		{"final relative escape", "profile.json", nativeAttemptProfileJSON, func(t *testing.T, root string) {
			_ = os.Remove(filepath.Join(root, "profile.json"))
			if err := os.Symlink("../outside.json", filepath.Join(root, "profile.json")); err != nil {
				t.Skipf("symlink: %v", err)
			}
		}, nil, true},
		{"contained relative symlink", "inside.json", nativeAttemptProfileJSON, func(t *testing.T, root string) {
			if err := os.Symlink("profile.json", filepath.Join(root, "inside.json")); err != nil {
				t.Skipf("symlink: %v", err)
			}
		}, nil, false},
		{"absolute link", "profile.json", nativeAttemptProfileJSON, func(t *testing.T, root string) {
			outside := filepath.Join(t.TempDir(), "profile.json")
			if err := os.WriteFile(outside, []byte(nativeAttemptProfileJSON(root)), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(filepath.Join(root, "profile.json")); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, filepath.Join(root, "profile.json")); err != nil {
				t.Skipf("symlink: %v", err)
			}
		}, nil, true},
		{"ancestor escape", "escape/profile.json", nativeAttemptProfileJSON, func(t *testing.T, root string) {
			if err := os.Symlink(t.TempDir(), filepath.Join(root, "escape")); err != nil {
				t.Skipf("symlink: %v", err)
			}
		}, nil, true},
		{"final escape trailing separator", "escape/", nativeAttemptProfileJSON, func(t *testing.T, root string) {
			if err := os.Symlink(t.TempDir(), filepath.Join(root, "escape")); err != nil {
				t.Skipf("symlink: %v", err)
			}
		}, nil, true},
		{"non regular leaf", "leaf", nativeAttemptProfileJSON, func(t *testing.T, root string) {
			if err := os.Mkdir(filepath.Join(root, "leaf"), 0o755); err != nil {
				t.Fatal(err)
			}
		}, nil, true},
		{"in place mutation", "profile.json", nativeAttemptProfileJSON, nil, func(t *testing.T, path string) {
			if err := os.WriteFile(path, []byte(`{}`), 0o644); err != nil {
				t.Fatal(err)
			}
		}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			if tt.path != "" && tt.name != "non regular leaf" {
				writeNativeAttemptProfile(t, filepath.Join(root, "profile.json"), tt.payload(root))
			}
			if tt.prepare != nil {
				tt.prepare(t, root)
			}
			handle, err := os.OpenRoot(root)
			if err != nil {
				t.Fatal(err)
			}
			defer handle.Close()
			profile, err := OpenNativeAttemptProfile(handle, tt.path)
			if tt.wantErr && tt.mutate == nil {
				if err == nil {
					t.Fatal("OpenNativeAttemptProfile succeeded")
				}
				return
			}
			if err != nil {
				t.Fatalf("OpenNativeAttemptProfile: %v", err)
			}
			defer profile.Close()
			if tt.mutate != nil {
				tt.mutate(t, filepath.Join(root, "profile.json"))
				err = profile.Revalidate()
			}
			if tt.wantErr != (err != nil) {
				t.Fatalf("Revalidate error = %v, want error=%v", err, tt.wantErr)
			}
		})
	}
}

func TestNativeAttemptProfileRetainsAcceptedHandle(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(*testing.T, string)
	}{
		{"leaf", func(t *testing.T, path string) {
			if err := os.Rename(path, path+".old"); err != nil {
				t.Fatal(err)
			}
			writeNativeAttemptProfile(t, path, "{}")
		}},
		{"ancestor", func(t *testing.T, path string) {
			if err := os.Rename(filepath.Dir(path), filepath.Dir(path)+".old"); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			writeNativeAttemptProfile(t, path, "{}")
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "profiles", "profile.json")
			if err := os.Mkdir(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			writeNativeAttemptProfile(t, path, nativeAttemptProfileJSON(root))
			handle, err := os.OpenRoot(root)
			if err != nil {
				t.Fatal(err)
			}
			defer handle.Close()
			profile, err := OpenNativeAttemptProfile(handle, "profiles/profile.json")
			if err != nil {
				t.Fatal(err)
			}
			defer profile.Close()
			tt.mutate(t, path)
			if err := profile.Revalidate(); err != nil || profile.Root != root {
				t.Fatalf("revalidation redirected: profile=%#v err=%v", profile, err)
			}
		})
	}
}

func nativeAttemptProfileJSON(root string) string {
	return fmt.Sprintf(`{"schema":"gentle-ai.native-attempt-profile/v1","root":%q,"cwd":%q,"dataSource":"src/data-source.ts","migrationGlob":"src/migrations/*.migration.{js,ts}"}`, root, root)
}

func writeNativeAttemptProfile(t *testing.T, path, payload string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
}
