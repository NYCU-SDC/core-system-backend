package oauthprovider

import (
	"NYCU-SDC/core-system-backend/internal/user"
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/oauth2"
	githuboauth "golang.org/x/oauth2/github"
	"io"
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
			Scopes: []string{
				"user:email",
				"read:user",
			},
			Endpoint: githuboauth.Endpoint,
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
	Company   string `json:"company"`
	Location  string `json:"location"`
	Bio       string `json:"bio"`
}

// GitHubEmail represents the response from GitHub's emails API
type GitHubEmail struct {
	Email      string `json:"email"`
	Primary    bool   `json:"primary"`
	Verified   bool   `json:"verified"`
	Visibility string `json:"visibility"`
}

// GetUserInfo fetches user information from GitHub's API
func (g *GitHubConfig) GetUserInfo(ctx context.Context, token *oauth2.Token) (user.User, user.Auth, string, error) {
	client := g.config.Client(ctx, token)

	// Get user info
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
	err = json.Unmarshal(body, &githubUser)
	if err != nil {
		return user.User{}, user.Auth{}, "", fmt.Errorf("failed to unmarshal GitHub user info: %v", err)
	}

	// Create User struct with GitHub data
	userInfo := user.User{
		Name:      pgtype.Text{String: githubUser.Name, Valid: githubUser.Name != ""},
		Username:  pgtype.Text{String: githubUser.Login, Valid: githubUser.Login != ""},
		AvatarUrl: pgtype.Text{String: githubUser.AvatarURL, Valid: githubUser.AvatarURL != ""},
		Role:      []string{"user"}, // Default role
	}

	// Create Auth struct with provider info
	authInfo := user.Auth{
		Provider:   "github",
		ProviderID: fmt.Sprintf("%d", githubUser.ID), // Use GitHub user ID as provider ID
	}

	return userInfo, authInfo, githubUser.Email, nil
}
