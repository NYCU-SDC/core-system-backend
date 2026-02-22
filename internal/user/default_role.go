package user

import "strings"

type roleStore struct {
	global map[string][]string
	org    map[string][]string
}

var store roleStore

func InitDefaultGlobalRole(m map[string]string) {
	store.global = buildStore(m)
}

func InitDefaultOrgRole(m map[string]string) {
	store.org = buildStore(m)
}

func DefaultGlobalRoles(email string) []string {
	if store.global == nil {
		return nil
	}
	return clone(store.global[normalize(email)])
}

func DefaultOrgRoles(email string) []string {
	if store.org == nil {
		return nil
	}
	return clone(store.org[normalize(email)])
}

func buildStore(cfg map[string]string) map[string][]string {
	result := make(map[string][]string, len(cfg))

	for email, role := range cfg {
		e := normalize(email)
		r := normalize(role)

		if e == "" || r == "" {
			continue
		}

		result[e] = append(result[e], r)
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
