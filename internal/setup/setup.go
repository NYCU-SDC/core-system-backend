package setup

type SetupConfig struct {
	Organizations []Organization `yaml:"organizations"`
	Users         []User         `yaml:"users"`
}

type Organization struct {
	Name        string `yaml:"name"`
	Slug        string `yaml:"slug"`
	Description string `yaml:"description"`
}

type User struct {
	Email             string      `yaml:"email"`
	GlobalRole        []string    `yaml:"global_role"`
	OrgMember         []OrgMember `yaml:"org_member"`
	AllowedOnboarding bool        `yaml:"allowed_onboarding"`
}

type OrgMember struct {
	Slug    string `yaml:"slug"`
	OrgRole string `yaml:"org_role"`
}
type AllowedOnboardingList map[string]struct{}
