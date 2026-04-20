package config

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Port                    int    `mapstructure:"PORT"`
	AppEnv                  string `mapstructure:"APP_ENV"`
	CORSOrigin              string `mapstructure:"CORS_ORIGIN"`
	RawTrustProxy           string `mapstructure:"TRUST_PROXY"`
	HTTPSRedirect           bool   `mapstructure:"HTTPS_REDIRECT"`
	DevHTTPS                bool   `mapstructure:"DEV_HTTPS"`
	DevHTTPSKeyPath         string `mapstructure:"DEV_HTTPS_KEY_PATH"`
	DevHTTPSCertPath        string `mapstructure:"DEV_HTTPS_CERT_PATH"`
	JSONBodyLimit           string `mapstructure:"JSON_BODY_LIMIT"`
	JWTSecret               string `mapstructure:"JWT_SECRET"`
	APIKey                  string `mapstructure:"GIT_AI_API_KEY"`
	APIKeys                 string `mapstructure:"GIT_AI_API_KEYS"`
	DefaultUserID           string `mapstructure:"DEFAULT_USER_ID"`
	DefaultUserEmail        string `mapstructure:"DEFAULT_USER_EMAIL"`
	DefaultUserName         string `mapstructure:"DEFAULT_USER_NAME"`
	DefaultUserRole         string `mapstructure:"DEFAULT_USER_ROLE"`
	DefaultPersonalOrgID    string `mapstructure:"DEFAULT_PERSONAL_ORG_ID"`
	DefaultOrgName          string `mapstructure:"DEFAULT_ORG_NAME"`
	DefaultOrgSlug          string `mapstructure:"DEFAULT_ORG_SLUG"`
	EncryptionMasterKey     string `mapstructure:"ENCRYPTION_MASTER_KEY"`
	CASEncryptionKey        string `mapstructure:"CAS_ENCRYPTION_KEY"`
	InitialAdminUsername    string `mapstructure:"INITIAL_ADMIN_USERNAME"`
	InitialAdminPassword    string `mapstructure:"INITIAL_ADMIN_PASSWORD"`
	DBHost                  string `mapstructure:"DB_HOST"`
	DBPort                  int    `mapstructure:"DB_PORT"`
	DBUser                  string `mapstructure:"DB_USER"`
	DBPassword              string `mapstructure:"DB_PASSWORD"`
	DBName                  string `mapstructure:"DB_NAME"`
	RawDatabaseURL          string `mapstructure:"DATABASE_URL"`
	DBSSL                   bool   `mapstructure:"DB_SSL"`
	DBSSLRejectUnauthorized bool   `mapstructure:"DB_SSL_REJECT_UNAUTHORIZED"`
	ReleaseStoragePath      string `mapstructure:"RELEASE_STORAGE_PATH"`
	ReleaseUploadToken      string `mapstructure:"RELEASE_UPLOAD_TOKEN"`
}

// TrustProxy parses the TRUST_PROXY value. It returns:
//   - bool(false) if empty, "false", "no", or "off"
//   - bool(true) if "true", "yes", or "on"
//   - int(n) if the value is a numeric string
//   - the raw string otherwise (e.g. "loopback")
func (c *Config) TrustProxy() any {
	v := strings.TrimSpace(c.RawTrustProxy)
	if v == "" {
		return false
	}

	lowered := strings.ToLower(v)

	switch lowered {
	case "true", "yes", "on":
		return true
	case "false", "no", "off":
		return false
	}

	if n, err := strconv.Atoi(v); err == nil {
		return n
	}

	return v
}

// DatabaseURL returns the configured DATABASE_URL if set, otherwise
// constructs a PostgreSQL connection string from individual DB_* fields.
func (c *Config) DatabaseURL() string {
	if u := strings.TrimSpace(c.RawDatabaseURL); u != "" {
		return u
	}

	credentials := url.PathEscape(c.DBUser)
	if c.DBPassword != "" {
		credentials = url.PathEscape(c.DBUser) + ":" + url.PathEscape(c.DBPassword)
	}

	params := url.Values{}
	if c.DBSSL {
		if c.DBSSLRejectUnauthorized {
			params.Set("sslmode", "verify-full")
		} else {
			params.Set("sslmode", "require")
		}
	} else {
		params.Set("sslmode", "disable")
	}

	query := params.Encode()
	dsn := fmt.Sprintf("postgresql://%s@%s:%d/%s", credentials, c.DBHost, c.DBPort, c.DBName)
	if query != "" {
		dsn += "?" + query
	}

	return dsn
}

// DescribeDatabaseTarget returns a human-readable string identifying the
// database connection target, suitable for startup log lines.
func (c *Config) DescribeDatabaseTarget() string {
	if strings.TrimSpace(c.RawDatabaseURL) != "" {
		return "postgres:DATABASE_URL"
	}

	return fmt.Sprintf("postgres://%s:%d/%s", c.DBHost, c.DBPort, c.DBName)
}

// Load reads configuration from environment variables and returns a Config
// with sensible defaults applied for any unset values.
func Load() (*Config, error) {
	v := viper.New()
	v.AutomaticEnv()

	v.SetDefault("PORT", 3000)
	v.SetDefault("APP_ENV", "development")
	v.SetDefault("CORS_ORIGIN", "http://localhost:3000")
	v.SetDefault("TRUST_PROXY", "false")
	v.SetDefault("HTTPS_REDIRECT", false)
	v.SetDefault("DEV_HTTPS", false)
	v.SetDefault("DEV_HTTPS_KEY_PATH", "certs/localhost-key.pem")
	v.SetDefault("DEV_HTTPS_CERT_PATH", "certs/localhost.pem")
	v.SetDefault("JSON_BODY_LIMIT", "2mb")
	v.SetDefault("JWT_SECRET", "")
	v.SetDefault("GIT_AI_API_KEY", "")
	v.SetDefault("GIT_AI_API_KEYS", "")
	v.SetDefault("DEFAULT_USER_ID", "00000000-0000-0000-0000-000000000001")
	v.SetDefault("DEFAULT_USER_EMAIL", "git-ai@example.local")
	v.SetDefault("DEFAULT_USER_NAME", "Git AI User")
	v.SetDefault("DEFAULT_USER_ROLE", "user")
	v.SetDefault("DEFAULT_PERSONAL_ORG_ID", "git-ai-local-org")
	v.SetDefault("DEFAULT_ORG_NAME", "Git AI Local")
	v.SetDefault("DEFAULT_ORG_SLUG", "git-ai-local")
	v.SetDefault("ENCRYPTION_MASTER_KEY", "")
	v.SetDefault("CAS_ENCRYPTION_KEY", "")
	v.SetDefault("INITIAL_ADMIN_USERNAME", "admin")
	v.SetDefault("INITIAL_ADMIN_PASSWORD", "")
	v.SetDefault("DB_HOST", "127.0.0.1")
	v.SetDefault("DB_PORT", 5432)
	v.SetDefault("DB_USER", "postgres")
	v.SetDefault("DB_PASSWORD", "")
	v.SetDefault("DB_NAME", "git_ai")
	v.SetDefault("DATABASE_URL", "")
	v.SetDefault("DB_SSL", false)
	v.SetDefault("DB_SSL_REJECT_UNAUTHORIZED", false)
	v.SetDefault("RELEASE_STORAGE_PATH", "/opt/git-ai/releases")
	v.SetDefault("RELEASE_UPLOAD_TOKEN", "")

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshalling config: %w", err)
	}

	return &cfg, nil
}

// ValidAPIKeys returns all configured API keys accepted for X-API-Key worker
// requests. GIT_AI_API_KEY is a single key; GIT_AI_API_KEYS is comma-separated.
func (c *Config) ValidAPIKeys() []string {
	seen := make(map[string]struct{})
	keys := make([]string, 0)

	add := func(value string) {
		for _, key := range strings.Split(value, ",") {
			trimmed := strings.TrimSpace(key)
			if trimmed == "" {
				continue
			}
			if _, ok := seen[trimmed]; ok {
				continue
			}
			seen[trimmed] = struct{}{}
			keys = append(keys, trimmed)
		}
	}

	add(c.APIKey)
	add(c.APIKeys)

	return keys
}
