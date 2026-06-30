package setup

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	databaseutil "github.com/NYCU-SDC/summer/pkg/database"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/moby/moby/api/types/container"
	"github.com/ory/dockertest/v4"
	"go.uber.org/zap"
)

func setupPostgres(pool dockertest.ClosablePool, logger *zap.Logger) (*pgxpool.Pool, string, func(), error) {
	ctx := context.Background()
	resource, err := pool.Run(ctx, "postgres",
		dockertest.WithTag("18"),
		dockertest.WithEnv([]string{
			"POSTGRES_PASSWORD=password",
			"POSTGRES_USER=postgres",
			"POSTGRES_DB=dbname",
		}),
		dockertest.WithHostConfig(func(config *container.HostConfig) {
			config.AutoRemove = true
			config.RestartPolicy = container.RestartPolicy{Name: "no"}
		}),
	)
	if err != nil {
		logger.Fatal("Could not start resource", zap.Error(err))
		return nil, "", nil, err
	}

	hostAndPort := resource.GetPort("5432/tcp")
	databaseURL := fmt.Sprintf("postgres://postgres:password@localhost:%s/dbname?sslmode=disable", hostAndPort)
	logger.Info("Launching Postgres", zap.String("url", databaseURL))

	retryCount := 0
	if err = pool.Retry(ctx, 120*time.Second, func() error {
		db, err := sql.Open("postgres", databaseURL)
		if err != nil {
			return err
		}
		defer func(db *sql.DB) {
			err := db.Close()
			if err != nil {
				log.Printf("Error closing database connection: %s", err)
			}
		}(db)

		err = db.Ping()
		if err != nil {
			retryCount++
			logger.Debug("Postgres not ready yet, retrying...", zap.Int("retry", retryCount))
			return err
		}

		return nil
	}); err != nil {
		logger.Fatal("Could not connect to resource", zap.Error(err))
	}

	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		logger.Fatal("Could not parse database URL", zap.Error(err))
		return nil, "", nil, err
	}

	dbPool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		logger.Fatal("Could not create database pool", zap.Error(err))
		return nil, "", nil, err
	}

	cleanup := func() {
		dbPool.Close()

		err := resource.Close(ctx)
		if err != nil {
			logger.Error("Failed to close resource", zap.Error(err))
		} else {
			logger.Info("Successfully closed resource")
		}
	}

	return dbPool, databaseURL, cleanup, nil
}

func setupPostgresWithMigrations(pool dockertest.ClosablePool, logger *zap.Logger, sourceURL string) (*pgxpool.Pool, string, func(), error) {
	dbPool, databaseURL, cleanup, err := setupPostgres(pool, logger)
	if err != nil {
		return nil, "", nil, err
	}

	err = databaseutil.MigrationUp(sourceURL, databaseURL, logger)
	if err != nil {
		cleanup()
		logger.Fatal("Failed to apply migrations", zap.Error(err))
		return nil, "", nil, err
	}

	return dbPool, databaseURL, cleanup, nil
}
