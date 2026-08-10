package main

import (
	"fmt"
	"log"
	"quartz/internal/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	dsn := "host=localhost user=postgres password=postgres dbname=postgres port=5432 sslmode=disable"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal(err)
	}

	var blocks []model.FileBlock
	db.Find(&blocks)
	for _, b := range blocks {
		fmt.Printf("Block %s, Size: %d, ObjectKey: %s\n", b.ID, b.Size, b.ObjectKey)
	}
    
    var users []model.User
    db.Find(&users)
    for _, u := range users {
        fmt.Printf("User %s, StorageUsed: %d\n", u.Email, u.StorageUsed)
    }
}
