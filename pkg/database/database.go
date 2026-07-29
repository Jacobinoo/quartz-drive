package database

import (
	"log"
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
		log.Printf("database is connecting... (attempt %d)", i)
		if db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
			Logger:                 logger.Default.LogMode(logger.Info),
			SkipDefaultTransaction: true,
			TranslateError:         true,
		}); err == nil {
			log.Println("database connected")
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
