package cache

import "github.com/redis/go-redis/v9"

type Config struct {
	Addr     string
	Password string
	DB       int
}

func New(cfg Config) *redis.Client {
	if cfg.Addr == "" {
		return nil
	}
	return redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
}
