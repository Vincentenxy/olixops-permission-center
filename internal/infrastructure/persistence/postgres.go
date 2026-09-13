package persistence

import (
	"errors"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func OpenPostgres(dsn string) (*gorm.DB, error) {
	if dsn == "" {
		return nil, nil
	}
	// GORM's default console logger can disclose DSNs or bound values.
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: gormlogger.Discard})
	if err != nil {
		return nil, errors.New("postgres initialization failed")
	}
	return db, nil
}
