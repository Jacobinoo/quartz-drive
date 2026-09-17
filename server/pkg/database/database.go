package database

import (
	"log"
	"os"
	"quartz/config"
	"quartz/internal/model"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func New() (db *gorm.DB, err error) {
	dsn := config.Cfg.GetDSN()
	for i := 1; i <= 3; i++ {
		log.Printf("database is co2nnecting... (attempt %d)", i)
		logLevel := logger.Info
		if config.Cfg.App.Env == "production" {
			logLevel = logger.Error
		}

		gormLogger := logger.New(
			log.New(os.Stdout, "\r\n", log.LstdFlags),
			logger.Config{
				SlowThreshold:             time.Second,
				LogLevel:                  logLevel,
				IgnoreRecordNotFoundError: true,
				Colorful:                  false,
				ParameterizedQueries:      true, // REDACT SENSITIVE VALUES
			},
		)

		if db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
			Logger:                 gormLogger,
			SkipDefaultTransaction: true,
			TranslateError:         true,
			PrepareStmt:            true,
		}); err == nil {
			log.Println("database connected")

			sqlDB, err := db.DB()
			if err == nil {
				sqlDB.SetMaxIdleConns(10)
				sqlDB.SetMaxOpenConns(50)
				sqlDB.SetConnMaxLifetime(time.Hour)
			}

			if config.Cfg.DB.AutoMigrate {
				log.Println("auto migration is running...")
				err = db.AutoMigrate(
					&model.User{},
					&model.UserKeyStore{},
					&model.GormRefreshToken{},
					&model.FileBlock{},
					&model.Node{},
					&model.Link{},
					&model.Share{},
					&model.ShareMember{},
					&model.Session{},
					&model.Upload{},
					&model.UploadChunk{},
					&model.PasswordResetToken{},
				)
			}
			return db, err
		}
		if i < 3 {
			time.Sleep(2 * time.Second)
		}
	}
	return db, err
}
