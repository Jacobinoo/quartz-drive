package config

import (
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/caarlos0/env/v11"
)

var BuildVersion = "unknown"

type (
	Config struct {
		Host         string `env:"HOST,required"`
		Port         string `env:"PORT,required"`
		CertFilePath string `env:"CERT_FILE_PATH"`
		KeyFilePath  string `env:"KEY_FILE_PATH"`

		App
		DB
		// GRPC
		KV
		JWT
		CRYPTO
		S3
		Security
		B2
		Sweeper
		Email
		RateLimits
	}

	RateLimits struct {
		DevicesIPRate    int `env:"RL_DEVICES_IP_RATE" envDefault:"300"`
		DevicesIPBurst   int `env:"RL_DEVICES_IP_BURST" envDefault:"100"`
		DevicesUserRate  int `env:"RL_DEVICES_USER_RATE" envDefault:"6"`
		DevicesUserBurst int `env:"RL_DEVICES_USER_BURST" envDefault:"30"`

		FilesIPRate    int `env:"RL_FILES_IP_RATE" envDefault:"300"`
		FilesIPBurst   int `env:"RL_FILES_IP_BURST" envDefault:"100"`
		FilesUserRate  int `env:"RL_FILES_USER_RATE" envDefault:"100"`
		FilesUserBurst int `env:"RL_FILES_USER_BURST" envDefault:"50"`

		KeysIPRate    int `env:"RL_KEYS_IP_RATE" envDefault:"300"`
		KeysIPBurst   int `env:"RL_KEYS_IP_BURST" envDefault:"100"`
		KeysUserRate  int `env:"RL_KEYS_USER_RATE" envDefault:"100"`
		KeysUserBurst int `env:"RL_KEYS_USER_BURST" envDefault:"50"`

		RefreshIPRate  int `env:"RL_REFRESH_IP_RATE" envDefault:"10"`
		RefreshIPBurst int `env:"RL_REFRESH_IP_BURST" envDefault:"5"`

		SigninIPRate  int `env:"RL_SIGNIN_IP_RATE" envDefault:"10"`
		SigninIPBurst int `env:"RL_SIGNIN_IP_BURST" envDefault:"10"`

		SignoutIPRate  int `env:"RL_SIGNOUT_IP_RATE" envDefault:"10"`
		SignoutIPBurst int `env:"RL_SIGNOUT_IP_BURST" envDefault:"5"`

		SignupIPRate  int `env:"RL_SIGNUP_IP_RATE" envDefault:"5"`
		SignupIPBurst int `env:"RL_SIGNUP_IP_BURST" envDefault:"2"`
	}

	Email struct {
		Key                   string `env:"EMAIL_KEY,required"`
		UpdatesFromSenderName string `env:"EMAIL_UPDATES_FROM_SENDER_NAME,required"`
		UpdatesVerifiedDomain string `env:"EMAIL_UPDATES_VERIFIED_DOMAIN,required"`

		SupportFromSenderName string `env:"EMAIL_SUPPORT_FROM_SENDER_NAME,required"`
		SupportVerifiedDomain string `env:"EMAIL_SUPPORT_VERIFIED_DOMAIN,required"`

		LocalDeliveryAddress string `env:"EMAIL_LOCAL_DELIVERY_ADDRESS"`
	}

	Security struct {
		CaptchaSecret    string `env:"SECURITY_CAPTCHA_SECRET,required"`
		CaptchaSiteKey   string `env:"SECURITY_CAPTCHA_SITE_KEY,required"`
		TurnstileSecret  string `env:"SECURITY_TURNSTILE_SECRET,required"`
		TurnstileSiteKey string `env:"SECURITY_TURNSTILE_SITE_KEY,required"`
	}

	App struct {
		Env         string `env:"APP_ENV,required"`
		Name        string `env:"APP_NAME,required"`
		Version     string `env:"APP_VERSION,required"`
		FrontendURL string `env:"FRONTEND_URL,required"`
		HealthToken string `env:"HEALTH_TOKEN,required"`
		SentryDSN   string `env:"SENTRY_DSN,required"`
	}

	DB struct {
		Host                      string `env:"DB_HOST,required"`
		User                      string `env:"DB_USER,required"`
		Port                      string `env:"DB_PORT,required"`
		Password                  string `env:"DB_PASSWORD,required"`
		Name                      string `env:"DB_NAME,required"`
		SSLMode                   string `env:"DB_SSLMODE,required"`
		AutoMigrate               bool   `env:"DB_AUTO_MIGRATE,required"`
		ConnectingApplicationName string `env:"APP_NAME,required"`
	}

	// GRPC struct {
	// 	Host string `env:"GRPC_HOST,required"`
	// 	Port string `env:"GRPC_PORT,required"`
	// }

	KV struct {
		URL string `env:"KV_URL,required"`
		Key string `env:"KV_KEY,required"`
	}

	JWT struct {
		SecretKey string `env:"JWT_PRIVATE_KEY_HEX,required"`
		PublicKey string `env:"JWT_PUBLIC_KEY_HEX,required"`
	}

	CRYPTO struct {
		KdfOpsLimit        int8   `env:"KDF_OPSLIMIT,required"`
		KdfMemLimit        int64  `env:"KDF_MEMLIMIT,required"`
		KdfAlg             int8   `env:"KDF_ALG,required"`
		EncryptionVersion  int16  `env:"ENC_VERSION,required"`
		OpaqueServerSetup  string `env:"OPAQUE_SERVER_SETUP,required"`
		EmailEncryptionKey string `env:"EMAIL_ENCRYPTION_KEY,required"`
		EmailHashSecretKey string `env:"EMAIL_HASH_SECRET_KEY,required"`
	}

	S3 struct {
		Endpoint           string `env:"S3_ENDPOINT,required"`
		AccessKeyID        string `env:"S3_ACCESS_KEY_ID,required"`
		SecretAccessKey    string `env:"S3_SECRET_ACCESS_KEY,required"`
		BucketName         string `env:"S3_BUCKET_NAME,required"`
		InsecureSkipVerify bool   `env:"S3_INSECURE_SKIP_VERIFY,required"`
		Secure             bool   `env:"S3_SECURE" envDefault:"true"`
	}

	B2 struct {
		ApplicationKeyID string `env:"B2_APPLICATION_KEY_ID,required"`
		ApplicationKey   string `env:"B2_APPLICATION_KEY,required"`
	}

	Sweeper struct {
		Disabled                  bool   `env:"DISABLE_SWEEPS" envDefault:"false"`
		HourlyInterval            string `env:"SWEEP_HOURLY_INTERVAL" envDefault:"1h"`
		DailyInterval             string `env:"SWEEP_DAILY_INTERVAL" envDefault:"24h"`
		CompletedInterval         string `env:"SWEEP_COMPLETED_INTERVAL" envDefault:"1h"`
		UploadSessionExpiresHours int    `env:"UPLOAD_SESSION_EXPIRES_HOURS" envDefault:"24"`
		TrashRetentionDays        int    `env:"TRASH_RETENTION_DAYS" envDefault:"30"`
	}
)

var Cfg Config

func (c *Config) validateApp() {
	//TODO
}

func (c *Config) validateCrypto() {
	//TODO
}
func (c *Config) validateJWT() {
	//TODO
}
func (c *Config) validateSweeper() {
	_, err := time.ParseDuration(c.Sweeper.HourlyInterval)
	if err != nil {
		slog.Error("failed to parse env vars", "error", err)
		os.Exit(1)
		return
	}
	_, err = time.ParseDuration(c.Sweeper.DailyInterval)
	if err != nil {
		slog.Error("failed to parse env vars", "error", err)
		os.Exit(1)
		return
	}
	_, err = time.ParseDuration(c.Sweeper.CompletedInterval)
	if err != nil {
		slog.Error("failed to parse env vars", "error", err)
		os.Exit(1)
		return
	}
	if c.Sweeper.UploadSessionExpiresHours <= 0 {
		slog.Error("Sweeper.UploadSessionExpiresHours must be higher than 0")
		os.Exit(1)
		return
	}
	if c.Sweeper.TrashRetentionDays <= 0 {
		slog.Error("Sweeper.TrashRetentionDays must be higher than 0")
		os.Exit(1)
		return
	}
}

func (c *Config) Init() {
	err := env.Parse(c)
	if err != nil {
		slog.Error("failed to parse env vars", "error", err)
		os.Exit(1)
		return
	}

	// Validate JWT Key to fail-fast
	privateKeyBytes, err := hex.DecodeString(c.JWT.SecretKey)
	if err != nil {
		slog.Error("JWT_PRIVATE_KEY_HEX is not a valid hex string", "error", err)
		os.Exit(1)
		return
	}
	if _, err := x509.ParseECPrivateKey(privateKeyBytes); err != nil {
		slog.Error("JWT_PRIVATE_KEY_HEX is not a valid ECDSA P-256 private key", "error", err)
		os.Exit(1)
		return
	}

	slog.Info("Environment loaded", "environment", c.App.Env, "version", c.App.Version)
}

func (db *DB) GetDSN() string {
	return fmt.Sprintf("host=%s user=%s password=%s dbname=%s sslmode=%s port=%s application_name=%s",
		db.Host, db.User, db.Password, db.Name, db.SSLMode, db.Port, db.ConnectingApplicationName)
}
