package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"

	"github.com/teamspace-app/backend/pkg/auth"
	"github.com/teamspace-app/backend/pkg/config"
	"github.com/teamspace-app/backend/pkg/kubernetes"
)

var (
	oauth2Config *oauth2.Config
	store        *sessions.CookieStore
	authHandler  *auth.AuthHandler
	k8sManager   *kubernetes.TeamspaceManager
	appConfig    *config.Config
)

// Logging response writer to capture status code
type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

// CORS middleware
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Log the request
		log.Printf("=== CORS: Handling %s request to %s", r.Method, r.URL.Path)
		log.Printf("=== CORS: Origin: %s", r.Header.Get("Origin"))

		// Get the origin from the request header
		origin := r.Header.Get("Origin")
		if strings.HasPrefix(origin, "http://localhost:") || strings.HasPrefix(origin, "https://localhost:") {
			// Allow any localhost origin as a fallback for development
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}

		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		// Handle preflight requests
		if r.Method == "OPTIONS" {
			log.Printf("=== CORS: Handling preflight request for path: %s", r.URL.Path)
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Request logger middleware
func requestLoggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Create a response writer wrapper to capture status code
		lrw := &loggingResponseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		next.ServeHTTP(lrw, r)

		duration := time.Since(start)
		log.Printf("=== RESPONSE: %s %s - Status: %d - Duration: %v",
			r.Method, r.URL.Path, lrw.statusCode, duration)
	})
}

// Authentication middleware
func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("=== AUTH CHECK: Checking authentication for %s %s", r.Method, r.URL.Path)

		// Skip auth check only for login, callback, and status endpoints
		if r.URL.Path == "/auth/login" || r.URL.Path == "/auth/callback" || r.URL.Path == "/auth/status" {
			log.Printf("=== AUTH CHECK: Skipping auth check for auth endpoint: %s", r.URL.Path)
			next.ServeHTTP(w, r)
			return
		}

		// Strict authentication check - no bypasses for any environment
		if !authHandler.IsAuthenticated(r) {
			log.Printf("=== AUTH CHECK: User is not authenticated - access denied")
			if r.URL.Path == "/" {
				log.Printf("=== AUTH CHECK: Redirecting to login page")
				http.Redirect(w, r, "/auth/login", http.StatusTemporaryRedirect)
				return
			}
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		log.Printf("=== AUTH CHECK: User is authenticated - access granted")
		next.ServeHTTP(w, r)
	}
}

func main() {
	// Define command line flags
	configPath := flag.String("config", "../config/config.json", "Path to configuration file")
	flag.Parse()

	// Load configuration
	var err error
	log.Printf("Loading configuration from %s", *configPath)
	appConfig, err = config.LoadFromFile(*configPath)
	if err != nil {
		log.Fatalf("Failed to loa config: %w", err)
	}

	// Initialize OAuth2 configuration
	oauth2Config = &oauth2.Config{
		ClientID:     appConfig.OAuth.GithubClientID,
		ClientSecret: appConfig.OAuth.GithubClientSecret,
		RedirectURL:  appConfig.OAuth.RedirectURL,
		Scopes:       []string{"read:org", "read:user", "user:email"},
		Endpoint:     github.Endpoint,
	}

	// Create the session store with keys from config
	store = sessions.NewCookieStore(
		[]byte(appConfig.Session.HashKey),
		[]byte(appConfig.Session.BlockKey),
	)

	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 7, // 7 days
		HttpOnly: false,
		Secure:   true, // Set to true in production with HTTPS
		SameSite: http.SameSiteLaxMode,
	}

	// Initialize auth handler
	authHandler = auth.NewAuthHandler(oauth2Config, appConfig, store, appConfig.App.AllowedTeams)

	// Initialize Kubernetes manager
	k8sManager, err = kubernetes.NewTeamspaceManager()
	if err != nil {
		log.Fatalf("Failed to initialize Kubernetes manager: %v", err)
	}

	r := mux.NewRouter()

	// Apply middlewares to the main router
	r.Use(corsMiddleware)
	r.Use(requestLoggerMiddleware)

	// Auth routes
	r.HandleFunc("/auth/login", authHandler.HandleLogin)
	r.HandleFunc("/auth/callback", authHandler.HandleCallback)
	r.HandleFunc("/auth/logout", authHandler.HandleLogout)
	r.HandleFunc("/auth/status", handleAuthStatus)

	// Protected routes
	apiRouter := r.PathPrefix("/api").Subrouter()

	// Ensure OPTIONS method is handled for API endpoints
	apiRouter.Methods("OPTIONS").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("=== API: Explicitly handling OPTIONS request for API endpoint")
		w.WriteHeader(http.StatusOK)
	})

	apiRouter.HandleFunc("/teamspaces", authMiddleware(handleListTeamspaces)).Methods("GET")
	apiRouter.HandleFunc("/teamspaces", authMiddleware(handleCreateTeamspace)).Methods("POST")
	apiRouter.HandleFunc("/teamspaces/{id}", authMiddleware(handleDeleteTeamspace)).Methods("DELETE")
	apiRouter.HandleFunc("/teamspaces/{id}/kubeconfig", authMiddleware(handleGetKubeconfig))

	// Serve static frontend files from the frontend/dist directory
	frontendPath := "/app/frontend/dist"
	// If the directory doesn't exist, fall back to the relative path for local development
	if _, err := os.Stat(frontendPath); os.IsNotExist(err) {
		frontendPath = "../frontend/dist"
	}
	log.Printf("=== SERVING: Frontend files from: %s", frontendPath)
	// Root route serving index.html
	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Always serve the frontend without checking auth
		// The frontend will handle showing login UI if needed
		http.ServeFile(w, r, frontendPath+"/index.html")
	})

	// Create a file server for the static files
	fileServer := http.FileServer(http.Dir(frontendPath))

	// Serve static assets without stripping prefix
	r.PathPrefix("/assets/").Handler(fileServer)

	// Handle favicon requests
	r.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, frontendPath+"/favicon.ico")
	})

	// Serve index.html for all other routes to support SPA routing
	r.PathPrefix("/").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip serving frontend for API and auth endpoints
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/auth/") {
			http.NotFound(w, r)
			return
		}

		// Check if the file exists in the dist directory
		requestedFile := frontendPath + r.URL.Path
		_, err := os.Stat(requestedFile)
		if err == nil {
			// If the file exists, serve it directly
			log.Printf("=== SERVING: Direct file access: %s", requestedFile)
			http.ServeFile(w, r, requestedFile)
			return
		}

		// For all other routes, serve index.html to support SPA client-side routing
		log.Printf("=== SERVING: Returning index.html for path: %s", r.URL.Path)
		http.ServeFile(w, r, frontendPath+"/index.html")
	})

	// Start the server
	serverAddr := fmt.Sprintf(":%d", appConfig.Server.Port)
	log.Printf("Backend API server starting on %s", serverAddr)
	log.Fatal(http.ListenAndServe(serverAddr, r))
}

func handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	log.Printf("=== AUTH STATUS: Checking for user %v", r.RemoteAddr)
	isAuth := authHandler.IsAuthenticated(r)
	log.Printf("=== AUTH STATUS: User is authenticated: %v", isAuth)

	// Create response object
	response := map[string]interface{}{
		"authenticated": isAuth,
	}

	// If authenticated, include username
	if isAuth {
		username, ok := authHandler.GetUsername(r)
		if ok {
			log.Printf("=== AUTH STATUS: Username from session: %s", username)
			response["username"] = username
		}
	}

	// Set headers
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleListTeamspaces(w http.ResponseWriter, r *http.Request) {
	log.Printf("=== LIST TEAMSPACES: Request received")

	// Get the username from session
	username, ok := authHandler.GetUsername(r)
	if !ok {
		log.Printf("=== LIST TEAMSPACES: No username found in session")
		http.Error(w, "Session error: no username", http.StatusUnauthorized)
		return
	}

	log.Printf("=== LIST TEAMSPACES: Listing teamspaces for user: %s", username)

	// List teamspaces by owner
	teamspaces, err := k8sManager.ListTeamspacesByOwner(username)
	if err != nil {
		log.Printf("=== LIST TEAMSPACES: Error listing teamspaces: %v", err)
		http.Error(w, "Unable to list teamspaces: "+err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("=== LIST TEAMSPACES: Returning %d teamspaces for user %s", len(teamspaces), username)

	// Set proper headers
	w.Header().Set("Content-Type", "application/json")
	// Encode response
	if err := json.NewEncoder(w).Encode(teamspaces); err != nil {
		log.Printf("=== LIST TEAMSPACES: Error encoding response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

func handleCreateTeamspace(w http.ResponseWriter, r *http.Request) {
	log.Printf("=== CREATE TEAMSPACE: Request received with method: %s", r.Method)
	log.Printf("=== CREATE TEAMSPACE: Request path: %s", r.URL.Path)
	log.Printf("=== CREATE TEAMSPACE: Content-Type: %s", r.Header.Get("Content-Type"))

	// Get the username from session
	username, ok := authHandler.GetUsername(r)
	if !ok {
		log.Printf("=== CREATE TEAMSPACE: No username found in session")
		http.Error(w, "Session error: no username", http.StatusUnauthorized)
		return
	}
	// Check if user already has 3 teamspaces (maximum allowed)
	existingTeamspaces, err := k8sManager.ListTeamspacesByOwner(username)
	if err != nil {
		log.Printf("=== CREATE TEAMSPACE: Error checking existing teamspaces: %v", err)
		http.Error(w, "Unable to check existing teamspaces: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if len(existingTeamspaces) >= 3 {
		log.Printf("=== CREATE TEAMSPACE: User %s already has maximum allowed teamspaces (%d)", username, len(existingTeamspaces))
		http.Error(w, "Maximum number of teamspaces (3) reached for this user", http.StatusForbidden)
		return
	}

	// Log content length
	log.Printf("=== CREATE TEAMSPACE: Content-Length: %d", r.ContentLength)

	// Read the request body
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("=== CREATE TEAMSPACE: Error reading request body: %v", err)
		http.Error(w, "Error reading request body", http.StatusBadRequest)
		return
	}

	// Log the raw request body
	log.Printf("=== CREATE TEAMSPACE: Raw request body: %s", string(bodyBytes))

	// Restore the body for further processing
	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	var data struct {
		Name                        string `json:"name"`
		InitialHostedClusterRelease string `json:"initialHostedClusterRelease"`
		FeatureSet                  string `json:"featureSet"`
	}
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		log.Printf("=== CREATE TEAMSPACE: Error decoding request body: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	log.Printf("=== CREATE TEAMSPACE: With name: %s and release: %s for user: %s", data.Name, data.InitialHostedClusterRelease, username)

	if data.Name == "" {
		log.Printf("=== CREATE TEAMSPACE: Empty name provided")
		http.Error(w, "Teamspace name cannot be empty", http.StatusBadRequest)
		return
	}

	teamspace, err := k8sManager.CreateTeamspace(data.Name, username, data.InitialHostedClusterRelease, data.FeatureSet)
	if err != nil {
		log.Printf("=== CREATE TEAMSPACE: Error creating teamspace: %v", err)
		http.Error(w, "Failed to create teamspace: "+err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("=== CREATE TEAMSPACE: Successfully created teamspace: %s for user: %s", teamspace.Name, username)
	// Set proper headers
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	// Encode response
	if err := json.NewEncoder(w).Encode(teamspace); err != nil {
		log.Printf("=== CREATE TEAMSPACE: Error encoding response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

func handleDeleteTeamspace(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	log.Printf("=== DELETE TEAMSPACE: With id: %s", id)

	if id == "" {
		log.Printf("=== DELETE TEAMSPACE: Empty ID provided")
		http.Error(w, "Teamspace ID cannot be empty", http.StatusBadRequest)
		return
	}

	// Get the username from session
	username, ok := authHandler.GetUsername(r)
	if !ok {
		log.Printf("=== DELETE TEAMSPACE: No username found in session")
		http.Error(w, "Session error: no username", http.StatusUnauthorized)
		return
	}

	// Check if user is the owner
	isOwner, err := k8sManager.IsTeamspaceOwner(id, username)
	if err != nil {
		log.Printf("=== DELETE TEAMSPACE: Error checking ownership: %v", err)
		http.Error(w, "Failed to check teamspace ownership: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if !isOwner {
		log.Printf("=== DELETE TEAMSPACE: User %s is not the owner of teamspace %s", username, id)
		http.Error(w, "You don't have permission to delete this teamspace", http.StatusForbidden)
		return
	}

	if err := k8sManager.DeleteTeamspace(id); err != nil {
		log.Printf("=== DELETE TEAMSPACE: Error deleting teamspace: %v", err)
		http.Error(w, "Failed to delete teamspace: "+err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("=== DELETE TEAMSPACE: Successfully deleted teamspace: %s", id)
	// Set headers
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNoContent)
}

func handleGetKubeconfig(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	log.Printf("=== GET KUBECONFIG: For teamspace: %s", id)

	// Get the username from session
	username, ok := authHandler.GetUsername(r)
	if !ok {
		log.Printf("=== GET KUBECONFIG: No username found in session")
		http.Error(w, "Session error: no username", http.StatusUnauthorized)
		return
	}

	// Check if user is the owner
	isOwner, err := k8sManager.IsTeamspaceOwner(id, username)
	if err != nil {
		log.Printf("=== GET KUBECONFIG: Error checking ownership: %v", err)
		http.Error(w, "Failed to check teamspace ownership: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if !isOwner {
		log.Printf("=== GET KUBECONFIG: User %s is not the owner of teamspace %s", username, id)
		http.Error(w, "You don't have permission to access this teamspace's kubeconfig", http.StatusForbidden)
		return
	}

	config, err := k8sManager.GetKubeconfig(id)
	if err != nil {
		log.Printf("=== GET KUBECONFIG: Error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("=== GET KUBECONFIG: Successfully retrieved kubeconfig for: %s", id)
	w.Header().Set("Content-Type", "application/yaml")
	w.Header().Set("Content-Disposition", "attachment; filename=kubeconfig.yaml")
	w.Write(config)
}
