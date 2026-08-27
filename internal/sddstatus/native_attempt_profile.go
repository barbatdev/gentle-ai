package sddstatus

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"github.com/gentleman-programming/gentle-ai/v2/internal/pathidentity"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	nativeAttemptProfileSchema       = "gentle-ai.native-attempt-profile/v1"
	nativeAttemptProfileDataSource   = "src/data-source.ts"
	nativeAttemptProfileMaximumBytes = 64 << 10
)

var errNativeAttemptProfile = errors.New("profile_invalid; rerun `gentle-ai sdd-attempt acquire` with a valid contained profile")

// NativeAttemptProfile is a concrete profile retained on its accepted file handle.
type NativeAttemptProfile struct {
	Root, CWD, DataSource, MigrationGlob string
	file                                 *os.File
	hash                                 [sha256.Size]byte
}

// OpenNativeAttemptProfile opens and validates a profile through the supplied root.
func OpenNativeAttemptProfile(root *os.Root, name string) (*NativeAttemptProfile, error) {
	if root == nil || name == "" || filepath.IsAbs(name) || filepath.Clean(name) != name || strings.HasSuffix(name, "/") || strings.HasSuffix(name, `\`) {
		return nil, errNativeAttemptProfile
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, errNativeAttemptProfile
	}
	profile, err := readNativeAttemptProfile(file)
	if err != nil {
		_ = file.Close()
		return nil, errNativeAttemptProfile
	}
	profile.file = file
	return profile, nil
}

func readNativeAttemptProfile(file *os.File) (*NativeAttemptProfile, error) {
	payload, err := io.ReadAll(io.LimitReader(file, nativeAttemptProfileMaximumBytes+1))
	if err != nil || len(payload) > nativeAttemptProfileMaximumBytes {
		return nil, errNativeAttemptProfile
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, errNativeAttemptProfile
	}
	profile, err := decodeNativeAttemptProfile(payload)
	if err != nil {
		return nil, errNativeAttemptProfile
	}
	profile.hash = sha256.Sum256(payload)
	return &profile, nil
}

func decodeNativeAttemptProfile(payload []byte) (NativeAttemptProfile, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return NativeAttemptProfile{}, errNativeAttemptProfile
	}
	values := map[string]*string{"schema": nil, "root": nil, "cwd": nil, "dataSource": nil, "migrationGlob": nil}
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return NativeAttemptProfile{}, errNativeAttemptProfile
		}
		name, ok := key.(string)
		valueSlot, known := values[name]
		if !ok || !known || valueSlot != nil {
			return NativeAttemptProfile{}, errNativeAttemptProfile
		}
		value := ""
		if err := decoder.Decode(&value); err != nil {
			return NativeAttemptProfile{}, errNativeAttemptProfile
		}
		values[name] = &value
	}
	if end, err := decoder.Token(); err != nil || end != json.Delim('}') {
		return NativeAttemptProfile{}, errNativeAttemptProfile
	}
	if _, err := decoder.Token(); err != io.EOF {
		return NativeAttemptProfile{}, errNativeAttemptProfile
	}
	if values["schema"] == nil || values["root"] == nil || values["cwd"] == nil || values["dataSource"] == nil || values["migrationGlob"] == nil ||
		*values["schema"] != nativeAttemptProfileSchema || *values["dataSource"] != nativeAttemptProfileDataSource ||
		!nativeAttemptProfileCanonicalPath(*values["root"]) || !nativeAttemptProfileCanonicalPath(*values["cwd"]) {
		return NativeAttemptProfile{}, errNativeAttemptProfile
	}
	return NativeAttemptProfile{Root: *values["root"], CWD: *values["cwd"], DataSource: *values["dataSource"], MigrationGlob: *values["migrationGlob"]}, nil
}

func nativeAttemptProfileCanonicalPath(path string) bool {
	resolved, err := filepath.EvalSymlinks(path)
	return err == nil && filepath.IsAbs(path) && filepath.Clean(path) == path && pathidentity.SameDirectory(path, resolved)
}

// Revalidate rejects in-place changes while retaining the accepted file handle.
func (profile *NativeAttemptProfile) Revalidate() error {
	if _, err := profile.file.Seek(0, io.SeekStart); err != nil {
		return errNativeAttemptProfile
	}
	payload, err := io.ReadAll(io.LimitReader(profile.file, nativeAttemptProfileMaximumBytes+1))
	if err != nil || len(payload) > nativeAttemptProfileMaximumBytes {
		return errNativeAttemptProfile
	}
	info, err := profile.file.Stat()
	if err != nil || !info.Mode().IsRegular() || sha256.Sum256(payload) != profile.hash {
		return errNativeAttemptProfile
	}
	return nil
}

func (profile *NativeAttemptProfile) Close() error { return profile.file.Close() }
