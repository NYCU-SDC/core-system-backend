package oauthprovider

import (
	"NYCU-SDC/core-system-backend/internal/user"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/oauth2"
)

type NYCUConfig struct {
	config *oauth2.Config
}

type NYCUOauth struct {
	ClientID     string `yaml:"client_id"     envconfig:"NYCU_OAUTH_CLIENT_ID"`
	ClientSecret string `yaml:"client_secret" envconfig:"NYCU_OAUTH_CLIENT_SECRET"`
}

func NewNYCUConfig(clientID, clientSecret, redirectURL string) *NYCUConfig {
	return &NYCUConfig{
		config: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Scopes:       []string{"profile"},
			Endpoint: oauth2.Endpoint{
				AuthURL:  "https://id.nycu.edu.tw/o/authorize/",
				TokenURL: "https://id.nycu.edu.tw/o/token/",
			},
		},
	}
}

func (n *NYCUConfig) Name() string {
	return "nycu"
}

func (n *NYCUConfig) Config() *oauth2.Config {
	return n.config
}

func (n *NYCUConfig) Exchange(ctx context.Context, code string) (*oauth2.Token, error) {
	return n.config.Exchange(ctx, code)
}

// NYCUProfile represents the response from NYCU's profile API
type NYCUProfile struct {
	Username string `json:"username"`
	Email    string `json:"email"`
}

// GetUserInfo fetches user information from NYCU's userinfo API
// Using the 'username' field from profile as recommended by NYCU for consistent user identification
func (n *NYCUConfig) GetUserInfo(ctx context.Context, token *oauth2.Token) (user.User, user.Auth, string, error) {
	client := n.config.Client(ctx, token)
	resp, err := client.Get("https://id.nycu.edu.tw/api/profile/")
	if err != nil {
		return user.User{}, user.Auth{}, "", fmt.Errorf("failed to get NYCU user info: %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return user.User{}, user.Auth{}, "", fmt.Errorf("failed to read NYCU user info: %v", err)
	}

	var nycuUser NYCUProfile
	err = json.Unmarshal(body, &nycuUser)
	if err != nil {
		return user.User{}, user.Auth{}, "", fmt.Errorf("failed to unmarshal NYCU user info: %v", err)
	}

	// Create User struct with NYCU data
	userInfo := user.User{
		Name:      pgtype.Text{String: nycuUser.Username, Valid: nycuUser.Username != ""},
		Username:  pgtype.Text{String: GetUsername(nycuUser.Email), Valid: nycuUser.Username != ""},
		AvatarUrl: pgtype.Text{String: "", Valid: false}, // NYCU doesn't provide avatar URL in profile API
		Role:      []string{"user"},                      // Default role
	}

	// Create Auth struct with provider info
	authInfo := user.Auth{
		Provider:   "nycu",
		ProviderID: nycuUser.Username, // Use 'username' field for consistent user identification
	}

	return userInfo, authInfo, nycuUser.Email, nil
}
