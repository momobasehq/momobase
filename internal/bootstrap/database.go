package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func OpenDatabase(cfg Config) (*gorm.DB, error) {
	gormLogger := logger.Default.LogMode(logger.Silent)
	conf := &gorm.Config{Logger: gormLogger, NowFunc: func() time.Time { return time.Now().UTC() }}
	switch cfg.DB.Type {
	case "sqlite":
		if err := os.MkdirAll(filepath.Dir(cfg.DB.Path), 0755); err != nil {
			return nil, err
		}
		return gorm.Open(sqlite.Open(cfg.DB.Path), conf)
	case "postgres":
		dsn := fmt.Sprintf(
			"host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=UTC",
			cfg.DB.Host,
			cfg.DB.User,
			cfg.DB.Password,
			cfg.DB.Name,
			cfg.DB.Port,
			cfg.DB.SSLMode,
		)
		return gorm.Open(postgres.Open(dsn), conf)
	case "mysql":
		dsn := fmt.Sprintf(
			"%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=UTC",
			cfg.DB.User,
			cfg.DB.Password,
			cfg.DB.Host,
			cfg.DB.Port,
			cfg.DB.Name,
		)
		return gorm.Open(mysql.Open(dsn), conf)
	default:
		return nil, fmt.Errorf("unsupported DB_TYPE %q", cfg.DB.Type)
	}
}
