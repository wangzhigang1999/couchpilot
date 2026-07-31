package mapping

import (
	"path/filepath"
	"strings"

	"github.com/wangzhigang1999/couchpilot/internal/core"
)

// ProfileMatcher owns the product rule that turns an application identity
// into a binding profile. Platform adapters only collect the identity.
type ProfileMatcher struct {
	profiles []core.AppProfile
}

func NewProfileMatcher(profiles []core.AppProfile) *ProfileMatcher {
	return &ProfileMatcher{profiles: append([]core.AppProfile(nil), profiles...)}
}

func (m *ProfileMatcher) Match(identity core.AppIdentity) string {
	processName := normalizeProcessName(identity.ProcessName)
	if processName == "" {
		processName = normalizeProcessName(baseName(identity.ExecutablePath))
	}
	path := normalizePath(identity.ExecutablePath)

	for _, profile := range m.profiles {
		if !matchesProcess(processName, profile.ProcessNames) {
			continue
		}
		if !matchesPath(path, profile.PathContains) {
			continue
		}
		return profile.Name
	}
	return "default"
}

func matchesProcess(processName string, candidates []string) bool {
	if len(candidates) == 0 {
		return true
	}
	for _, candidate := range candidates {
		if processName == normalizeProcessName(candidate) {
			return true
		}
	}
	return false
}

func matchesPath(path string, candidates []string) bool {
	if len(candidates) == 0 {
		return true
	}
	for _, candidate := range candidates {
		if strings.Contains(path, normalizePath(candidate)) {
			return true
		}
	}
	return false
}

func normalizeProcessName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	return strings.TrimSuffix(name, ".exe")
}

func normalizePath(path string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(path), `\`, "/"))
}

func baseName(path string) string {
	return filepath.Base(strings.ReplaceAll(path, `\`, "/"))
}
