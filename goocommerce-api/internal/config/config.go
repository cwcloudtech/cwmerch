// Package config reads goocommerce's configuration from the environment.
package config

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"time"

	"goocommerce-api/internal/models"
	"goocommerce-api/internal/utils"
)

type Config struct {
	Port       string
	MaxWorkers int

	// MySQL/MariaDB shared with WordPress/WooCommerce.
	DBHost, DBPort, DBUser, DBPassword, DBName, DBPrefix string
	DBPoolSize                                           int

	JWTSecret          string
	CorsEnabled        bool
	CorsAllowedOrigins []string
	Version            string
	ManifestPath       string
	OtelEndpoint       string
	OtelProto          string
	MaxImageSize       int64
	MaxUploadSize      int64
	APIBaseURL         string
	UIBaseURL          string
	// UIInternalURL is where the API fetches the UI's index.html for SEO
	// rendering (inside the compose network: http://ui).
	UIInternalURL string
	// ShopURL is the storefront's public URL (permalinks, canonical links);
	// defaults to UIBaseURL.
	ShopURL string
	// UploadsDir/UploadsURL: wp-content/uploads (a volume shared with
	// WordPress when it runs alongside), served at /v1/media when no URL is given.
	UploadsDir string
	UploadsURL string
	// PasswordHash is "bcrypt" (WordPress >= 6.8) or "phpass" (older WordPress).
	PasswordHash string
	// APIOnly: no storefront/back-office routes, goocommerce only as an AI
	// agent (+ API keys, MFA, accounting exports) next to WooCommerce.
	APIOnly bool

	OIDCGoogleClientID       string
	OIDCGoogleClientSecret   string
	OIDCGithubClientID       string
	OIDCGithubClientSecret   string
	OIDCKeycloakBaseURL      string
	OIDCKeycloakClientID     string
	OIDCKeycloakClientSecret string
	OIDCKeycloakGroups       []string

	CWCloudAPIURL        string
	CWCloudAPIKey        string
	CWCloudContactFormID string
	EmailFrom            string
	ConfirmationEmailTTL time.Duration
	ActivationMode       string
	DiscordToken         string
	IPInfoToken          string

	AIDownloadDir      string
	AIDownloadTTL      time.Duration
	AIDownloadMaxBytes int64
	AIDownloadSweep    time.Duration
}

func envInt(name string, def int) int {
	n, err := strconv.Atoi(strings.TrimSpace(os.Getenv(name)))
	if err != nil || n <= 0 {
		return def
	}
	return n
}

func envInt64(name string, def int64) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(os.Getenv(name)), 10, 64)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

func envDuration(name string, def time.Duration) time.Duration {
	d, err := time.ParseDuration(strings.TrimSpace(os.Getenv(name)))
	if err != nil || d <= 0 {
		return def
	}
	return d
}

func Load() Config {
	activation := utils.GetEnv("GOOCOMMERCE_ACTIVATION_MODE", models.ActivationModeAuto)
	if !models.IsValidActivationMode(activation) {
		activation = models.ActivationModeAuto
	}
	manifest := utils.GetEnv("GOOCOMMERCE_MANIFEST_PATH", "manifest.json")
	ui := utils.GetBaseUrlFromEnvWithFallback("GOOCOMMERCE_UI_URL", "http://localhost:3000")
	api := utils.GetBaseUrlFromEnvWithFallback("GOOCOMMERCE_API_URL", "http://localhost:8080")
	uploadsDir := os.Getenv("GOOCOMMERCE_UPLOADS_DIR")
	uploadsURL := strings.TrimRight(os.Getenv("GOOCOMMERCE_UPLOADS_URL"), "/")
	if uploadsURL == "" && uploadsDir != "" {
		uploadsURL = api + "/v1/media"
	}
	return Config{
		Port:                     utils.GetEnv("PORT", "8080"),
		MaxWorkers:               envInt("MAX_WORKERS", 5),
		DBHost:                   utils.GetEnv("MYSQL_HOST", "localhost"),
		DBPort:                   utils.GetEnv("MYSQL_PORT", "3306"),
		DBUser:                   utils.GetEnv("MYSQL_USER", "wordpress"),
		DBPassword:               os.Getenv("MYSQL_PASSWORD"),
		DBName:                   utils.GetEnv("MYSQL_DATABASE", "wordpress"),
		DBPrefix:                 utils.GetEnv("WP_TABLE_PREFIX", "wp_"),
		DBPoolSize:               envInt("MYSQL_POOL_SIZE", 10),
		JWTSecret:                os.Getenv("JWT_SECRET"),
		CorsEnabled:              utils.IsTrue(os.Getenv("GOOCOMMERCE_CORS_ENABLED")),
		CorsAllowedOrigins:       strings.Split(utils.GetEnv("GOOCOMMERCE_CORS_ALLOWED_ORIGINS", "*"), ","),
		Version:                  versionFromManifest(manifest, utils.GetEnv("APP_VERSION", "1.0.0")),
		ManifestPath:             manifest,
		OtelEndpoint:             os.Getenv("GOOCOMMERCE_OTEL_ENDPOINT"),
		OtelProto:                utils.GetEnv("GOOCOMMERCE_OTEL_PROTO", "otlp/grpc"),
		MaxImageSize:             envInt64("GOOCOMMERCE_MAX_IMAGE_SIZE", 2<<20),
		MaxUploadSize:            envInt64("GOOCOMMERCE_MAX_UPLOAD_SIZE", 10<<20),
		APIBaseURL:               api,
		UIBaseURL:                ui,
		UIInternalURL:            strings.TrimRight(utils.GetEnv("GOOCOMMERCE_UI_INTERNAL_URL", "http://ui"), "/"),
		ShopURL:                  utils.GetBaseUrlFromEnvWithFallback("GOOCOMMERCE_SHOP_URL", ui),
		UploadsDir:               uploadsDir,
		UploadsURL:               uploadsURL,
		PasswordHash:             utils.GetEnv("GOOCOMMERCE_PASSWORD_HASH", "bcrypt"),
		APIOnly:                  utils.IsTrue(os.Getenv("GOOCOMMERCE_API_ONLY")),
		OIDCGoogleClientID:       os.Getenv("GOOCOMMERCE_OIDC_GOOGLE_CLIENT_ID"),
		OIDCGoogleClientSecret:   os.Getenv("GOOCOMMERCE_OIDC_GOOGLE_CLIENT_SECRET"),
		OIDCGithubClientID:       os.Getenv("GOOCOMMERCE_OIDC_GITHUB_CLIENT_ID"),
		OIDCGithubClientSecret:   os.Getenv("GOOCOMMERCE_OIDC_GITHUB_CLIENT_SECRET"),
		OIDCKeycloakBaseURL:      utils.GetBaseUrlFromEnv("GOOCOMMERCE_OIDC_KEYCLOAK_BASE_URL"),
		OIDCKeycloakClientID:     os.Getenv("GOOCOMMERCE_OIDC_KEYCLOAK_CLIENT_ID"),
		OIDCKeycloakClientSecret: os.Getenv("GOOCOMMERCE_OIDC_KEYCLOAK_CLIENT_SECRET"),
		OIDCKeycloakGroups:       utils.SplitList(os.Getenv("GOOCOMMERCE_OIDC_KEYCLOAK_GROUPS")),
		CWCloudAPIURL:            utils.GetBaseUrlFromEnvWithFallback("CWCLOUD_API_URL", "https://api.cwcloud.tech"),
		CWCloudAPIKey:            os.Getenv("CWCLOUD_API_KEY"),
		CWCloudContactFormID:     os.Getenv("CWCLOUD_CONTACT_FORM_ID"),
		EmailFrom:                utils.GetEnv("GOOCOMMERCE_MAIL_FROM", "noreply@cwcloud.tech"),
		ConfirmationEmailTTL:     time.Duration(envInt("GOOCOMMERCE_CONFIRMATION_EMAIL_EXPIRATION", 24)) * time.Hour,
		ActivationMode:           activation,
		DiscordToken:             os.Getenv("DISCORD_TOKEN"),
		IPInfoToken:              os.Getenv("GOOCOMMERCE_IPINFO_TOKEN"),
		AIDownloadDir:            utils.GetEnv("GOOCOMMERCE_AI_DOWNLOAD_DIR", "storage"),
		AIDownloadTTL:            envDuration("GOOCOMMERCE_AI_DOWNLOAD_TTL", 30*time.Minute),
		AIDownloadMaxBytes:       envInt64("GOOCOMMERCE_AI_DOWNLOAD_MAX_BYTES", 25<<20),
		AIDownloadSweep:          envDuration("GOOCOMMERCE_AI_DOWNLOAD_SWEEP_INTERVAL", 5*time.Minute),
	}
}

func versionFromManifest(path, fallback string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return fallback
	}
	var m struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &m); err != nil || utils.IsBlank(m.Version) {
		return fallback
	}
	return m.Version
}
