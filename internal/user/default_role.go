package user

import (
	"strings"
)

type roleStore struct {
	global map[string][]string // multi-role
	org    map[string]string   // single-role
}

var store roleStore

func InitDefaultGlobalRole(cfg string) {
	store.global = parseMulti(cfg)
}

func InitDefaultOrgRole(cfg string) {
	store.org = parseSingle(cfg)
}

func DefaultGlobalRoles(email string) []string {
	if store.global == nil {
		return nil
	}
	return clone(store.global[normalize(email)])
}

func DefaultOrgRole(email string) (string, bool) {
	if store.org == nil {
		return "", false
	}

	role, ok := store.org[normalize(email)]
	return role, ok
}

func parseMulti(cfg string) map[string][]string {
	result := make(map[string][]string)

	lines := strings.Split(cfg, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		email := normalize(parts[0])
		role := normalize(parts[1])

		result[email] = append(result[email], role)
	}

	return result
}

func parseSingle(cfg string) map[string]string {
	result := make(map[string]string)

	lines := strings.Split(cfg, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		email := normalize(parts[0])
		role := normalize(parts[1])

		// overwrite if duplicated (last wins)
		result[email] = role
	}

	return result
}

func normalize(s string) string {
	return strings.TrimSpace(strings.ToLower(s))
}

func clone(src []string) []string {
	if src == nil {
		return nil
	}

	dst := make([]string, len(src))
	copy(dst, src)
	return dst
}
