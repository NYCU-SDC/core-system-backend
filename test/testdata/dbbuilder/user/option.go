package userbuilder

type Option func(*FactoryParams)

func WithName(name string) Option {
	return func(p *FactoryParams) {
		p.Name = name
	}
}

func WithUsername(username string) Option {
	return func(p *FactoryParams) {
		p.Username = username
	}
}

func WithAvatarURL(avatarURL string) Option {
	return func(p *FactoryParams) {
		p.AvatarURL = avatarURL
	}
}

func WithRole(role []string) Option {
	return func(p *FactoryParams) {
		p.Role = role
	}
}

func WithIsOnboarded(isOnboarded bool) Option {
    return func(p *FactoryParams) {
        p.IsOnboarded = isOnboarded
    }
}