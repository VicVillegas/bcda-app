package database

import (
	"database/sql"
	"log"
	"os"
	"sync"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	_ "github.com/lib/pq"
)

// Variable substitution to support testing.
var LogFatal = log.Fatal

// Use singleton pattern to ensure one of each DB connection per instance
var gormInstance *gorm.DB
var dbInstance *sql.DB
var gormOnce sync.Once
var dbOnce synce.Once

func GetDbConnection() *sql.DB {
	dbOnce.Do(func() {
		databaseURL := os.Getenv("DATABASE_URL")
		dbIntance, err := sql.Open("postgres", databaseURL)
		if err != nil {
			LogFatal(err)
		}
		pingErr := dbInstance.Ping()
		if pingErr != nil {
			LogFatal(pingErr)
		}
	})
	return dbInstance
}

func GetGORMDbConnection() *gorm.DB {
        gormOnce.Do(func() {
		databaseURL := os.Getenv("DATABASE_URL")
		gormInstance, err := gorm.Open("postgres", databaseURL)
		if err != nil {
			LogFatal(err)
		}
		pingErr := gormInstance.DB().Ping()
		if pingErr != nil {
			LogFatal(pingErr)
		}
	})
	return gormInstance
}
