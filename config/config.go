package config

import (
	"fmt"
	"log"

	"github.com/caarlos0/env/v11"
)

type (
	Config struct {
		Host         string `env:"HOST,required"`
		Port         string `env:"PORT,required"`
		CertFilePath string `env:"CERT_FILE_PATH,required"`
		KeyFilePath  string `env:"KEY_FILE_PATH,required"`

		App
		DB
		// GRPC
		KV
		JWT
		CRYPTO
		S3
		B2
		Sweeper
		Email
	}

	Email struct {
		Key string `env:"EMAIL_KEY,required"`
	}

	App struct {
		Env        string `env:"APP_ENV,required"`
		Name       string `env:"APP_NAME,required"`
		Version    string `env:"APP_VERSION,required"`
		FrontendURL string `env:"FRONTEND_URL,required"`
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
		KdfOpsLimit       int8   `env:"KDF_OPSLIMIT,required"`
		KdfMemLimit       int64  `env:"KDF_MEMLIMIT,required"`
		KdfAlg            int8   `env:"KDF_ALG,required"`
		EncryptionVersion int16  `env:"ENC_VERSION,required"`
		OpaqueServerSetup string `env:"OPAQUE_SERVER_SETUP,required"`
	}

	S3 struct {
		Endpoint           string `env:"S3_ENDPOINT,required"`
		AccessKeyID        string `env:"S3_ACCESS_KEY_ID,required"`
		SecretAccessKey    string `env:"S3_SECRET_ACCESS_KEY,required"`
		BucketName         string `env:"S3_BUCKET_NAME,required"`
		InsecureSkipVerify bool   `env:"S3_INSECURE_SKIP_VERIFY,required"`
	}

	B2 struct {
		ApplicationKeyID string `env:"B2_APPLICATION_KEY_ID,required"`
		ApplicationKey   string `env:"B2_APPLICATION_KEY,required"`
	}

	Sweeper struct {
		Disabled                 bool   `env:"DISABLE_SWEEPS" envDefault:"false"`
		HourlyInterval           string `env:"SWEEP_HOURLY_INTERVAL" envDefault:"1h"`
		DailyInterval            string `env:"SWEEP_DAILY_INTERVAL" envDefault:"24h"`
		CompletedInterval        string `env:"SWEEP_COMPLETED_INTERVAL" envDefault:"1h"`
		UploadSessionExpiresHours int    `env:"UPLOAD_SESSION_EXPIRES_HOURS" envDefault:"24"`
		TrashRetentionDays       int    `env:"TRASH_RETENTION_DAYS" envDefault:"30"`
	}
)

var Cfg Config

func (c *Config) Init() {
	err := env.Parse(c)
	if err != nil {
		log.Fatalf("failed to parse env vars: %v", err)
		return
	}

	fmt.Printf("Environment \"%s\" loaded.\n", c.App.Env)
	fmt.Printf("Version: %s\n", c.App.Version)
}

func (db *DB) GetDSN() string {
	return fmt.Sprintf("host=%s user=%s password=%s dbname=%s sslmode=%s port=%s application_name=%s",
		db.Host, db.User, db.Password, db.Name, db.SSLMode, db.Port, db.ConnectingApplicationName)
}
