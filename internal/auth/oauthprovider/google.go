package oauthprovider

import (
	"NYCU-SDC/core-system-backend/internal/user"
	"context"
	"encoding/json"
	"io"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

type GoogleConfig struct {
	config *oauth2.Config
}

type GoogleOauth struct {
	ClientID     string `yaml:"client_id"     envconfig:"GOOGLE_OAUTH_CLIENT_ID"`
	ClientSecret string `yaml:"client_secret" envconfig:"GOOGLE_OAUTH_CLIENT_SECRET"`
}

func NewGoogleConfig(clientID, clientSecret, redirectURL string) *GoogleConfig {
	return &GoogleConfig{
		config: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Scopes: []string{
				"openid",
				"https://www.googleapis.com/auth/userinfo.email",
				"https://www.googleapis.com/auth/userinfo.profile",
			},
			Endpoint: google.Endpoint,
		},
	}
}

func (g *GoogleConfig) Name() string {
	return "google"
}

func (g *GoogleConfig) Config() *oauth2.Config {
	return g.config
}

func (g *GoogleConfig) Exchange(ctx context.Context, code string) (*oauth2.Token, error) {
	return g.config.Exchange(ctx, code)
}

func GetUsername(email string) string {
	return strings.Split(email, "@")[0]
}

// GoogleUserInfo represents the response from Google's userinfo API
type GoogleUserInfo struct {
	Sub           string `json:"sub"`
	Name          string `json:"name"`
	GivenName     string `json:"given_name"`
	FamilyName    string `json:"family_name"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Picture       string `json:"picture"`
}

// GetUserInfo fetches user information from Google's JWT token (ID token)
// Using the 'sub' field from JWT token as recommended by Google for consistent user identification
func (g *GoogleConfig) GetUserInfo(ctx context.Context, token *oauth2.Token) (user.User, user.Auth, error) {
	client := g.config.Client(ctx, token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v3/userinfo")
	if err != nil {
		return user.User{}, user.Auth{}, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return user.User{}, user.Auth{}, err
	}

	var googleUser GoogleUserInfo
	if err := json.Unmarshal(body, &googleUser); err != nil {
		return user.User{}, user.Auth{}, err
	}

	// Create User struct with Google data
	userInfo := user.User{
		Name:      pgtype.Text{String: googleUser.Name, Valid: googleUser.Name != ""},
		Username:  pgtype.Text{String: GetUsername(googleUser.Email), Valid: googleUser.Email != ""},
		AvatarUrl: pgtype.Text{String: googleUser.Picture, Valid: googleUser.Picture != ""},
		Role:      []string{"user"}, // Default role
	}

	// Create Auth struct with provider info
	authInfo := user.Auth{
		Provider:   "google",
		ProviderID: googleUser.Sub, // Use 'sub' field for consistent user identification
	}

	return userInfo, authInfo, nil
}

// GetEmailFromToken extracts the email from the OAuth token response
func (g *GoogleConfig) GetEmailFromToken(ctx context.Context, token *oauth2.Token) (string, error) {
	client := g.config.Client(ctx, token)
	response, err := client.Get("https://www.googleapis.com/oauth2/v3/userinfo")
	if err != nil {
		return "", err
	}
	defer func() {
		_ = response.Body.Close()
	}()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}

	var googleUser GoogleUserInfo
	if err := json.Unmarshal(body, &googleUser); err != nil {
		return "", err
	}

	return googleUser.Email, nil
}
