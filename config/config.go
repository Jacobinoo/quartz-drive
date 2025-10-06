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
		GRPC
		JWT
	}

	App struct {
		Env     string `env:"APP_ENV,required"`
		Name    string `env:"APP_NAME,required"`
		Version string `env:"APP_VERSION,required"`
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

	GRPC struct {
		Host string `env:"GRPC_HOST,required"`
		Port string `env:"GRPC_PORT,required"`
	}

	JWT struct {
		SecretKey string `env:"JWT_PRIVATE_KEY_HEX,required"`
		PublicKey string `env:"JWT_PUBLIC_KEY_HEX,required"`
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
