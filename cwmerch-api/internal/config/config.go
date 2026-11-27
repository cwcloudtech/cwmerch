// Package config reads cwmerch's configuration from the environment.
package config

import (
	"encoding/json"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"cwmerch-api/internal/db"
	"cwmerch-api/internal/models"
	"cwmerch-api/internal/utils"
)

type Config struct {
	Port       string
	MaxWorkers int

	// MySQL/MariaDB shared with WordPress/WooCommerce.
	// DBType is "mysql" or "postgres" (normalized from DB_TYPE).
	DBType                                               string
	DBHost, DBPort, DBUser, DBPassword, DBName, DBPrefix string
	DBSSLMode                                            string
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
	// ShopURL is the storefront's public URL (permalinks, canonical links);
	// defaults to UIBaseURL.
	ShopURL string
	// UploadsDir/UploadsURL: wp-content/uploads (a volume shared with
	// WordPress when it runs alongside), served at /v1/media when no URL is given.
	UploadsDir string
	UploadsURL string
	// PasswordHash is "bcrypt" (WordPress >= 6.8) or "phpass" (older WordPress).
	PasswordHash string
	// APIOnly: no storefront/back-office routes, cwmerch only as an AI
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
	// SecretKey seals payment providers' credentials (base64 32 bytes).
	SecretKey string
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
	activation := utils.GetEnv("CWMERCH_ACTIVATION_MODE", models.ActivationModeAuto)
	if !models.IsValidActivationMode(activation) {
		activation = models.ActivationModeAuto
	}
	manifest := utils.GetEnv("CWMERCH_MANIFEST_PATH", "manifest.json")
	ui := utils.GetBaseUrlFromEnvWithFallback("CWMERCH_UI_URL", "http://localhost:3000")
	api := utils.GetBaseUrlFromEnvWithFallback("CWMERCH_API_URL", "http://localhost:8080")
	uploadsDir := os.Getenv("CWMERCH_UPLOADS_DIR")
	uploadsURL := strings.TrimRight(os.Getenv("CWMERCH_UPLOADS_URL"), "/")
	if uploadsURL == "" && uploadsDir != "" {
		uploadsURL = api + "/v1/media"
	}
	dbType, err := db.NormalizeType(firstEnv("mysql", "DB_TYPE"))
	if err != nil {
		slog.Warn("unknown DB_TYPE, using mysql", "type", os.Getenv("DB_TYPE"))
		dbType = db.MySQL
	}
	return Config{
		Port:                     utils.GetEnv("PORT", "8080"),
		MaxWorkers:               envInt("MAX_WORKERS", 5),
		DBType:                   dbType,
		DBHost:                   firstEnv("localhost", "DB_HOST", "MYSQL_HOST"),
		DBPort:                   firstEnv(db.DefaultPort(dbType), "DB_PORT", "MYSQL_PORT"),
		DBUser:                   firstEnv("wordpress", "DB_USER", "MYSQL_USER"),
		DBPassword:               firstEnv("", "DB_PASSWORD", "MYSQL_PASSWORD"),
		DBName:                   firstEnv("wordpress", "DB_NAME", "MYSQL_DATABASE"),
		DBPrefix:                 firstEnv("wp_", "DB_TABLE_PREFIX", "WP_TABLE_PREFIX"),
		DBPoolSize:               envInt("DB_POOL_SIZE", envInt("MYSQL_POOL_SIZE", 10)),
		DBSSLMode:                firstEnv("disable", "DB_SSLMODE"),
		JWTSecret:                os.Getenv("JWT_SECRET"),
		CorsEnabled:              utils.IsTrue(os.Getenv("CWMERCH_CORS_ENABLED")),
		CorsAllowedOrigins:       strings.Split(utils.GetEnv("CWMERCH_CORS_ALLOWED_ORIGINS", "*"), ","),
		Version:                  versionFromManifest(manifest, utils.GetEnv("APP_VERSION", "1.0.0")),
		ManifestPath:             manifest,
		OtelEndpoint:             os.Getenv("CWMERCH_OTEL_ENDPOINT"),
		OtelProto:                utils.GetEnv("CWMERCH_OTEL_PROTO", "otlp/grpc"),
		MaxImageSize:             envInt64("CWMERCH_MAX_IMAGE_SIZE", 2<<20),
		MaxUploadSize:            envInt64("CWMERCH_MAX_UPLOAD_SIZE", 10<<20),
		APIBaseURL:               api,
		UIBaseURL:                ui,
		ShopURL:                  utils.GetBaseUrlFromEnvWithFallback("CWMERCH_SHOP_URL", ui),
		UploadsDir:               uploadsDir,
		UploadsURL:               uploadsURL,
		PasswordHash:             utils.GetEnv("CWMERCH_PASSWORD_HASH", "bcrypt"),
		APIOnly:                  utils.IsTrue(os.Getenv("CWMERCH_API_ONLY")),
		OIDCGoogleClientID:       os.Getenv("CWMERCH_OIDC_GOOGLE_CLIENT_ID"),
		OIDCGoogleClientSecret:   os.Getenv("CWMERCH_OIDC_GOOGLE_CLIENT_SECRET"),
		OIDCGithubClientID:       os.Getenv("CWMERCH_OIDC_GITHUB_CLIENT_ID"),
		OIDCGithubClientSecret:   os.Getenv("CWMERCH_OIDC_GITHUB_CLIENT_SECRET"),
		OIDCKeycloakBaseURL:      utils.GetBaseUrlFromEnv("CWMERCH_OIDC_KEYCLOAK_BASE_URL"),
		OIDCKeycloakClientID:     os.Getenv("CWMERCH_OIDC_KEYCLOAK_CLIENT_ID"),
		OIDCKeycloakClientSecret: os.Getenv("CWMERCH_OIDC_KEYCLOAK_CLIENT_SECRET"),
		OIDCKeycloakGroups:       utils.SplitList(os.Getenv("CWMERCH_OIDC_KEYCLOAK_GROUPS")),
		CWCloudAPIURL:            utils.GetBaseUrlFromEnvWithFallback("CWCLOUD_API_URL", "https://api.cwcloud.tech"),
		CWCloudAPIKey:            os.Getenv("CWCLOUD_API_KEY"),
		CWCloudContactFormID:     os.Getenv("CWCLOUD_CONTACT_FORM_ID"),
		EmailFrom:                utils.GetEnv("CWMERCH_MAIL_FROM", "noreply@cwcloud.tech"),
		ConfirmationEmailTTL:     time.Duration(envInt("CWMERCH_CONFIRMATION_EMAIL_EXPIRATION", 24)) * time.Hour,
		ActivationMode:           activation,
		DiscordToken:             os.Getenv("DISCORD_TOKEN"),
		IPInfoToken:              os.Getenv("CWMERCH_IPINFO_TOKEN"),
		AIDownloadDir:            utils.GetEnv("CWMERCH_AI_DOWNLOAD_DIR", "storage"),
		AIDownloadTTL:            envDuration("CWMERCH_AI_DOWNLOAD_TTL", 30*time.Minute),
		AIDownloadMaxBytes:       envInt64("CWMERCH_AI_DOWNLOAD_MAX_BYTES", 25<<20),
		SecretKey:                os.Getenv("CWMERCH_SECRET_KEY"),
		AIDownloadSweep:          envDuration("CWMERCH_AI_DOWNLOAD_SWEEP_INTERVAL", 5*time.Minute),
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

// firstEnv is the first non-empty variable among names (the generic DB_*
// ones first, then the former MYSQL_* names), else def.
func firstEnv(def string, names ...string) string {
	for _, n := range names {
		if v := strings.TrimSpace(os.Getenv(n)); v != "" {
			return v
		}
	}
	return def
}
