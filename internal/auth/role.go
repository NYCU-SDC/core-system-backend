package auth

type Role int

const (
	RoleMember Role = iota
	RoleAdmin
)

func (r Role) Allow(required Role) bool {
	return r >= required
}

func ParseRole(s string) (Role, bool) {
	switch s {
	case "member":
		return RoleMember, true
	case "admin":
		return RoleAdmin, true
	default:
		return 0, false
	}
}

func (r Role) String() string {
	switch r {
	case RoleMember:
		return "member"
	case RoleAdmin:
		return "admin"
	default:
		return "unknown"
	}
}
