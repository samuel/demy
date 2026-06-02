package service

import (
	"fmt"
	"net/url"
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the Quoin server YAML configuration.
type Config struct {
	BaseURL              string         `yaml:"base_url"`
	Listen               string         `yaml:"listen"`
	LogLevel             string         `yaml:"log_level"`
	TRMNLDeviceAPIPrefix string         `yaml:"trmnl_device_api_prefix"`
	Database             DatabaseConfig `yaml:"database"`
	TLS                  TLSConfig      `yaml:"tls"`
	WebAuthn             WebAuthnConfig `yaml:"webauthn"`
	MQTT                 MQTTConfig     `yaml:"mqtt"`
}

// MQTTConfig holds connection settings for the optional Home Assistant MQTT bridge.
// When BrokerURL is empty the bridge is disabled.
type MQTTConfig struct {
	BrokerURL string `yaml:"broker_url"` // e.g. "tcp://localhost:1883"
	Username  string `yaml:"username"`
	Password  string `yaml:"password"`
	BaseTopic string `yaml:"base_topic"` // default "quoin"
	HAPrefix  string `yaml:"ha_prefix"`  // default "homeassistant"
}

// DatabaseConfig holds the SQLite database path.
type DatabaseConfig struct {
	Path string `yaml:"path"`
}

// TLSConfig controls TLS termination mode.
type TLSConfig struct {
	Mode     string     `yaml:"mode"` // "off", "manual", "auto"
	CertFile string     `yaml:"cert_file"`
	KeyFile  string     `yaml:"key_file"`
	ACME     ACMEConfig `yaml:"acme"`
}

// ACMEConfig holds Let's Encrypt / ACME settings for automatic TLS.
type ACMEConfig struct {
	Domains  []string `yaml:"domains"`
	Email    string   `yaml:"email"`
	CacheDir string   `yaml:"cache_dir"`

	// Challenge selects the ACME challenge type: "http" (default) or "dns".
	// Use "dns" when port 80 is not reachable from the internet (e.g. behind NAT).
	Challenge string `yaml:"challenge"`

	// Route53 holds configuration for the Route53 DNS-01 provider.
	// Required when challenge is "dns".
	Route53 Route53Config `yaml:"route53"`

	// Staging uses the Let's Encrypt staging CA instead of production.
	// Certificates issued by the staging CA are not trusted by browsers but
	// don't count against rate limits. Useful for testing configuration.
	Staging bool `yaml:"staging"`
}

// Route53Config holds AWS credentials and settings for the Route53 DNS-01 provider.
// All fields are optional; when omitted the standard AWS credential chain is used
// (AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY env vars, ~/.aws/credentials, IAM role).
type Route53Config struct {
	Region          string `yaml:"region"`
	AccessKeyID     string `yaml:"access_key_id"`
	SecretAccessKey string `yaml:"secret_access_key"`
	Profile         string `yaml:"profile"`          // AWS named profile (optional)
	HostedZoneID    string `yaml:"hosted_zone_id"`   // pin to a specific zone ID (optional)
	PropagationWait string `yaml:"propagation_wait"` // e.g. "2m"; uses lego default when empty
}

// WebAuthnConfig holds WebAuthn / passkey identity settings.
type WebAuthnConfig struct {
	RPID          string `yaml:"rp_id"`
	RPDisplayName string `yaml:"rp_display_name"`
}

// LoadConfig reads and parses the YAML config file at path, then applies env var overrides.
//
// Supported env vars:
//
//	QUOIN_BASE_URL
//	QUOIN_LISTEN
//	QUOIN_LOG_LEVEL
//	QUOIN_DATABASE_PATH
//	QUOIN_TLS_MODE
//	QUOIN_TLS_CERT_FILE
//	QUOIN_TLS_KEY_FILE
//	QUOIN_ACME_DOMAINS   (comma-separated)
//	QUOIN_ACME_EMAIL
//	QUOIN_ACME_CACHE_DIR
//	QUOIN_WEBAUTHN_RP_ID
//	QUOIN_WEBAUTHN_RP_DISPLAY_NAME
//	QUOIN_MQTT_BROKER_URL
//	QUOIN_MQTT_USERNAME
//	QUOIN_MQTT_PASSWORD
//	QUOIN_MQTT_BASE_TOPIC
//	QUOIN_MQTT_HA_PREFIX
func LoadConfig(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	applyEnvOverrides(&cfg)

	return &cfg, nil
}

// applyEnvOverrides sets config fields from environment variables when non-empty.
func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("QUOIN_BASE_URL"); v != "" {
		cfg.BaseURL = v
	}
	if v := os.Getenv("QUOIN_TRMNL_DEVICE_API_PREFIX"); v != "" {
		cfg.TRMNLDeviceAPIPrefix = v
	}
	if v := os.Getenv("QUOIN_LISTEN"); v != "" {
		cfg.Listen = v
	}
	if v := os.Getenv("QUOIN_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
	if v := os.Getenv("QUOIN_DATABASE_PATH"); v != "" {
		cfg.Database.Path = v
	}
	if v := os.Getenv("QUOIN_TLS_MODE"); v != "" {
		cfg.TLS.Mode = v
	}
	if v := os.Getenv("QUOIN_TLS_CERT_FILE"); v != "" {
		cfg.TLS.CertFile = v
	}
	if v := os.Getenv("QUOIN_TLS_KEY_FILE"); v != "" {
		cfg.TLS.KeyFile = v
	}
	if v := os.Getenv("QUOIN_ACME_EMAIL"); v != "" {
		cfg.TLS.ACME.Email = v
	}
	if v := os.Getenv("QUOIN_ACME_CACHE_DIR"); v != "" {
		cfg.TLS.ACME.CacheDir = v
	}
	if v := os.Getenv("QUOIN_ACME_CHALLENGE"); v != "" {
		cfg.TLS.ACME.Challenge = v
	}
	if v := os.Getenv("QUOIN_ACME_ROUTE53_REGION"); v != "" {
		cfg.TLS.ACME.Route53.Region = v
	}
	if v := os.Getenv("QUOIN_ACME_ROUTE53_ACCESS_KEY_ID"); v != "" {
		cfg.TLS.ACME.Route53.AccessKeyID = v
	}
	if v := os.Getenv("QUOIN_ACME_ROUTE53_SECRET_ACCESS_KEY"); v != "" {
		cfg.TLS.ACME.Route53.SecretAccessKey = v
	}
	if v := os.Getenv("QUOIN_ACME_ROUTE53_HOSTED_ZONE_ID"); v != "" {
		cfg.TLS.ACME.Route53.HostedZoneID = v
	}
	if v := os.Getenv("QUOIN_WEBAUTHN_RP_ID"); v != "" {
		cfg.WebAuthn.RPID = v
	}
	if v := os.Getenv("QUOIN_WEBAUTHN_RP_DISPLAY_NAME"); v != "" {
		cfg.WebAuthn.RPDisplayName = v
	}
	if v := os.Getenv("QUOIN_MQTT_BROKER_URL"); v != "" {
		cfg.MQTT.BrokerURL = v
	}
	if v := os.Getenv("QUOIN_MQTT_USERNAME"); v != "" {
		cfg.MQTT.Username = v
	}
	if v := os.Getenv("QUOIN_MQTT_PASSWORD"); v != "" {
		cfg.MQTT.Password = v
	}
	if v := os.Getenv("QUOIN_MQTT_BASE_TOPIC"); v != "" {
		cfg.MQTT.BaseTopic = v
	}
	if v := os.Getenv("QUOIN_MQTT_HA_PREFIX"); v != "" {
		cfg.MQTT.HAPrefix = v
	}
}

// RPID returns the WebAuthn RP ID. If not explicitly set in config, it is derived
// from the hostname of base_url.
func (c *Config) RPID() string {
	if c.WebAuthn.RPID != "" {
		return c.WebAuthn.RPID
	}
	u, err := url.Parse(c.BaseURL)
	if err != nil || u.Hostname() == "" {
		return c.BaseURL
	}
	return u.Hostname()
}

// RPOrigin returns the WebAuthn origin (scheme + host from base_url).
func (c *Config) RPOrigin() string {
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return c.BaseURL
	}
	return u.Scheme + "://" + u.Host
}
