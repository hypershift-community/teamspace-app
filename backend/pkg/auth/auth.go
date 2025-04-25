package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"

	"encoding/base64"

	"github.com/gorilla/sessions"
	"github.com/teamspace-app/backend/pkg/config"
	"golang.org/x/oauth2"
)

const (
	sessionName = "teamspace-session"
)

type AuthHandler struct {
	config    *oauth2.Config
	appConfig *config.Config
	store     *sessions.CookieStore
	allowed   []string // List of allowed GitHub teams
}

func NewAuthHandler(config *oauth2.Config, appConfig *config.Config, store *sessions.CookieStore, allowedTeams []string) *AuthHandler {
	return &AuthHandler{
		config:    config,
		appConfig: appConfig,
		store:     store,
		allowed:   allowedTeams,
	}
}

func (h *AuthHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	// Generate a random state string
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		log.Printf("Failed to generate random state: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	state := base64.StdEncoding.EncodeToString(b)

	// Get or create a new session
	session, _ := h.store.New(r, sessionName)

	// Store the state in the session
	session.Values["state"] = state
	session.Options.MaxAge = 300 // 5 minutes

	if err := session.Save(r, w); err != nil {
		log.Printf("Failed to save session: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Always redirect to GitHub for authorization
	url := h.config.AuthCodeURL(state, oauth2.AccessTypeOnline)
	log.Printf("=== AUTH: Redirecting to GitHub for authorization at URL: %s", url)
	log.Printf("=== AUTH: Using Client ID: %s", h.config.ClientID)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func (h *AuthHandler) HandleCallback(w http.ResponseWriter, r *http.Request) {
	// Get the code and state from the callback
	code := r.URL.Query().Get("code")
	incomingState := r.URL.Query().Get("state")

	log.Printf("=== AUTH CALLBACK: Received code and state. State: %s", incomingState)

	if code == "" || incomingState == "" {
		log.Printf("=== AUTH CALLBACK: Missing required parameters")
		http.Error(w, "Missing required parameters", http.StatusBadRequest)
		return
	}

	// Get the session
	session, err := h.store.Get(r, sessionName)
	if err != nil {
		log.Printf("=== AUTH CALLBACK: Failed to get session: %v", err)
		http.Error(w, "Session error", http.StatusInternalServerError)
		return
	}

	// Verify the state
	storedState, ok := session.Values["state"].(string)
	if !ok {
		log.Printf("=== AUTH CALLBACK: No state found in session")
		http.Error(w, "Invalid state", http.StatusBadRequest)
		return
	}

	log.Printf("=== AUTH CALLBACK: Comparing states - Stored: %s, Incoming: %s", storedState, incomingState)

	if storedState != incomingState {
		log.Printf("=== AUTH CALLBACK: State mismatch: stored=%s, incoming=%s", storedState, incomingState)

		http.Error(w, "Invalid state", http.StatusBadRequest)
		return
	}

	// Exchange the code for a token
	token, err := h.config.Exchange(r.Context(), code)
	if err != nil {
		log.Printf("=== AUTH CALLBACK: Failed to exchange code: %v", err)
		http.Error(w, "Failed to exchange code", http.StatusInternalServerError)
		return
	}

	// Get user information from GitHub
	username, err := h.getUserInfo(r.Context(), token.AccessToken)
	if err != nil {
		log.Printf("=== AUTH CALLBACK: Failed to get user info: %v", err)
		http.Error(w, "Failed to get user info", http.StatusInternalServerError)
		return
	}

	// Get user's GitHub teams
	teams, err := h.getUserTeams(r.Context(), token.AccessToken)
	if err != nil {
		log.Printf("=== AUTH CALLBACK: Failed to get teams: %v", err)
		http.Error(w, "Failed to get teams", http.StatusInternalServerError)
		return
	}

	// Check if user is allowed
	if !h.isUserAllowed(teams) {
		log.Printf("=== AUTH CALLBACK: User %s is not authorized", username)
		http.Error(w, "User not authorized", http.StatusForbidden)
		return
	}

	// Set authentication in session
	session.Values["authenticated"] = true
	session.Values["access_token"] = token.AccessToken
	session.Values["username"] = username
	log.Printf("=== AUTH CALLBACK: Setting username in session: %s", username)

	// Save the session
	if err := session.Save(r, w); err != nil {
		log.Printf("=== AUTH CALLBACK: Failed to save session: %v", err)
		http.Error(w, "Failed to save session", http.StatusInternalServerError)
		return
	}

	// Redirect to the frontend URL instead of root path
	log.Printf("=== AUTH CALLBACK: Authentication successful, redirecting to frontend: %s", h.appConfig.App.FrontendURL)
	http.Redirect(w, r, h.appConfig.App.FrontendURL, http.StatusTemporaryRedirect)
}

func (h *AuthHandler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	log.Printf("=== LOGOUT: Starting logout process")

	// Get the session
	session, err := h.store.Get(r, sessionName)
	if err != nil {
		log.Printf("=== LOGOUT: Error getting session: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Clear all session values
	for k := range session.Values {
		log.Printf("=== LOGOUT: Deleting session key: %v", k)
		delete(session.Values, k)
	}

	// Explicitly set authentication to false
	session.Values["authenticated"] = false

	// Set MaxAge to -1 to delete the cookie
	session.Options.MaxAge = -1
	session.Options.Path = "/"

	// Save the session to apply changes
	err = session.Save(r, w)
	if err != nil {
		log.Printf("=== LOGOUT: Error saving session: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Add a cache control header to prevent caching
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	log.Printf("=== LOGOUT: Session cleared successfully")

	// Always return a JSON response for AJAX compatibility
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Logged out successfully",
	})
}

func (h *AuthHandler) IsAuthenticated(r *http.Request) bool {
	log.Printf("=== STRICT AUTH CHECK: Verifying authentication from session")

	session, err := h.store.Get(r, sessionName)
	if err != nil {
		log.Printf("Session error in IsAuthenticated: %v", err)
		return false
	}

	authenticated, ok := session.Values["authenticated"].(bool)
	if !ok {
		log.Printf("No authentication status found in session")
		return false
	}

	// Regardless of environment, strictly check the session value
	log.Printf("=== STRICT AUTH CHECK: Authentication value from session: %v", authenticated)
	return authenticated
}

// GetUsername retrieves the GitHub username from the session if authenticated
func (h *AuthHandler) GetUsername(r *http.Request) (string, bool) {
	session, err := h.store.Get(r, sessionName)
	if err != nil {
		log.Printf("Session error in GetUsername: %v", err)
		return "", false
	}

	// First check if the user is authenticated
	authenticated, ok := session.Values["authenticated"].(bool)
	if !ok || !authenticated {
		return "", false
	}

	// Get the username from the session
	username, ok := session.Values["username"].(string)
	if !ok || username == "" {
		log.Printf("No username found in session")
		return "", false
	}

	return username, true
}

// getUserInfo gets the user's GitHub username
func (h *AuthHandler) getUserInfo(ctx context.Context, accessToken string) (string, error) {
	client := &http.Client{}
	userReq, err := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/user", nil)
	if err != nil {
		return "", err
	}

	userReq.Header.Set("Authorization", "Bearer "+accessToken)
	userReq.Header.Set("Accept", "application/vnd.github.v3+json")

	userResp, err := client.Do(userReq)
	if err != nil {
		return "", err
	}
	defer userResp.Body.Close()

	var user struct {
		Login string `json:"login"`
	}
	if err := json.NewDecoder(userResp.Body).Decode(&user); err != nil {
		return "", err
	}

	log.Printf("=== AUTH: Got GitHub username: %s", user.Login)
	return user.Login, nil
}

func (h *AuthHandler) getUserTeams(ctx context.Context, accessToken string) ([]string, error) {
	// First get the user's login
	client := &http.Client{}
	userReq, err := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/user", nil)
	if err != nil {
		return nil, fmt.Errorf("error creating user request: %v", err)
	}

	userReq.Header.Set("Authorization", "Bearer "+accessToken)
	userReq.Header.Set("Accept", "application/vnd.github.v3+json")

	userResp, err := client.Do(userReq)
	if err != nil {
		return nil, fmt.Errorf("error fetching user: %v", err)
	}
	defer userResp.Body.Close()

	var user struct {
		Login string `json:"login"`
	}
	if err := json.NewDecoder(userResp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("error decoding user response: %v", err)
	}

	log.Printf("=== AUTH: User login: %s", user.Login)

	// Check if GitHub org is configured
	if h.appConfig.App.GithubOrg == "" {
		log.Printf("=== AUTH: No GitHub org configured, returning empty teams list")
		return []string{}, nil
	}

	// Check if user is a member of the organization
	orgReq, err := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf("https://api.github.com/orgs/%s/members/%s", h.appConfig.App.GithubOrg, user.Login), nil)
	if err != nil {
		return nil, fmt.Errorf("error creating org membership request: %v", err)
	}

	orgReq.Header.Set("Authorization", "Bearer "+accessToken)
	orgReq.Header.Set("Accept", "application/vnd.github.v3+json")

	orgResp, err := client.Do(orgReq)
	if err != nil {
		return nil, fmt.Errorf("error checking org membership: %v", err)
	}
	defer orgResp.Body.Close()

	log.Printf("=== AUTH: Org membership status: %d", orgResp.StatusCode)

	// If user is not a member of the org, return empty teams list
	if orgResp.StatusCode != http.StatusNoContent {
		log.Printf("=== AUTH: User is not a member of org %s", h.appConfig.App.GithubOrg)
		return []string{}, nil
	}

	// Fetch the user's teams in the organization
	teamsReq, err := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf("https://api.github.com/user/teams?per_page=100"), nil)
	if err != nil {
		return nil, fmt.Errorf("error creating teams request: %v", err)
	}

	teamsReq.Header.Set("Authorization", "Bearer "+accessToken)
	teamsReq.Header.Set("Accept", "application/vnd.github.v3+json")

	teamsResp, err := client.Do(teamsReq)
	if err != nil {
		return nil, fmt.Errorf("error fetching teams: %v", err)
	}
	defer teamsResp.Body.Close()

	var teamsData []struct {
		Name string `json:"name"`
		Org  struct {
			Login string `json:"login"`
		} `json:"organization"`
	}

	if err := json.NewDecoder(teamsResp.Body).Decode(&teamsData); err != nil {
		return nil, fmt.Errorf("error decoding teams response: %v", err)
	}

	// Filter teams to only include those from the configured organization
	var teams []string
	for _, team := range teamsData {
		if team.Org.Login == h.appConfig.App.GithubOrg {
			teamName := team.Name
			teams = append(teams, teamName)
			log.Printf("=== AUTH: Found team: %s in org %s", teamName, h.appConfig.App.GithubOrg)
		}
	}

	log.Printf("=== AUTH: User belongs to %d teams in org %s", len(teams), h.appConfig.App.GithubOrg)

	if len(teams) == 0 {
		log.Printf("=== AUTH: User is a member of the organization but belongs to no teams")
	}

	return teams, nil
}

func (h *AuthHandler) isUserAllowed(teams []string) bool {
	log.Printf("=== AUTH: Checking if user is allowed. Teams: %v, Allowed teams: %v", teams, h.allowed)

	// If no teams are specified in config, allow all authenticated org members
	if h.allowed == nil || len(h.allowed) == 0 {
		log.Printf("=== AUTH: No teams specified in config, allowing all authenticated org members")
		return true
	}

	// If user has no teams, deny access
	if len(teams) == 0 {
		log.Printf("=== AUTH: User has no teams, denying access")
		return false
	}

	// Check if user belongs to any of the allowed teams
	for _, team := range teams {
		for _, allowed := range h.allowed {
			if team == allowed {
				log.Printf("=== AUTH: User is in allowed team: %s", team)
				return true
			}
		}
	}

	log.Printf("=== AUTH: User's teams don't match any allowed teams, denying access")
	return false
}
