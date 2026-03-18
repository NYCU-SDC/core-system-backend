package oauthprovider

import (
	"NYCU-SDC/core-system-backend/internal/user"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
)

type GitHubConfig struct {
	config *oauth2.Config
}

type GitHubOauth struct {
	ClientID     string `yaml:"client_id"     envconfig:"GITHUB_OAUTH_CLIENT_ID"`
	ClientSecret string `yaml:"client_secret" envconfig:"GITHUB_OAUTH_CLIENT_SECRET"`
}

func NewGitHubConfig(clientID, clientSecret, redirectURL string) *GitHubConfig {
	return &GitHubConfig{
		config: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Scopes:       []string{"read:user", "user:email"},
			Endpoint:     github.Endpoint,
		},
	}
}

func (g *GitHubConfig) Name() string {
	return "github"
}

func (g *GitHubConfig) Config() *oauth2.Config {
	return g.config
}

func (g *GitHubConfig) Exchange(ctx context.Context, code string) (*oauth2.Token, error) {
	return g.config.Exchange(ctx, code)
}

// GitHubUserInfo represents the response from GitHub's user API
type GitHubUserInfo struct {
	ID        int    `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
}

// GitHubEmail represents a single entry from GitHub's emails API
type GitHubEmail struct {
	Email    string `json:"email"`
	Primary  bool   `json:"primary"`
	Verified bool   `json:"verified"`
}

// GetUserInfo fetches user information from GitHub's user API.
// The 'id' field (numeric GitHub user ID) is used as the stable provider identifier.
// If the user's public email is not set, the primary verified email is fetched from
// the /user/emails endpoint.
func (g *GitHubConfig) GetUserInfo(ctx context.Context, token *oauth2.Token) (user.User, user.Auth, string, error) {
	client := g.config.Client(ctx, token)

	// Fetch user profile
	resp, err := client.Get("https://api.github.com/user")
	if err != nil {
		return user.User{}, user.Auth{}, "", fmt.Errorf("failed to get GitHub user info: %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return user.User{}, user.Auth{}, "", fmt.Errorf("failed to read GitHub user info: %v", err)
	}

	var githubUser GitHubUserInfo
	if err = json.Unmarshal(body, &githubUser); err != nil {
		return user.User{}, user.Auth{}, "", fmt.Errorf("failed to unmarshal GitHub user info: %v", err)
	}

	email := githubUser.Email

	// If the public profile email is empty, fetch from the emails endpoint
	if email == "" {
		email, err = g.getPrimaryEmail(ctx, token)
		if err != nil {
			return user.User{}, user.Auth{}, "", err
		}
	}

	displayName := githubUser.Name
	if displayName == "" {
		displayName = githubUser.Login
	}

	// Create User struct with GitHub data
	userInfo := user.User{
		Name:      pgtype.Text{String: displayName, Valid: displayName != ""},
		Username:  pgtype.Text{String: githubUser.Login, Valid: githubUser.Login != ""},
		AvatarUrl: pgtype.Text{String: githubUser.AvatarURL, Valid: githubUser.AvatarURL != ""},
		Role:      []string{"user"}, // Default role
	}

	// Create Auth struct with provider info
	authInfo := user.Auth{
		Provider:   "github",
		ProviderID: fmt.Sprintf("%d", githubUser.ID), // Use numeric ID for stable identification
	}

	return userInfo, authInfo, email, nil
}

// getPrimaryEmail fetches the primary verified email from GitHub's /user/emails endpoint.
func (g *GitHubConfig) getPrimaryEmail(ctx context.Context, token *oauth2.Token) (string, error) {
	client := g.config.Client(ctx, token)

	resp, err := client.Get("https://api.github.com/user/emails")
	if err != nil {
		return "", fmt.Errorf("failed to get GitHub user emails: %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read GitHub user emails: %v", err)
	}

	var emails []GitHubEmail
	if err = json.Unmarshal(body, &emails); err != nil {
		return "", fmt.Errorf("failed to unmarshal GitHub user emails: %v", err)
	}

	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email, nil
		}
	}

	return "", nil
}
