package setup

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	databaseutil "github.com/NYCU-SDC/summer/pkg/database"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"go.uber.org/zap"
)

func setupPostgres(pool *dockertest.Pool, logger *zap.Logger) (*pgxpool.Pool, string, func(), error) {
	resource, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "postgres",
		Tag:        "18",
		Env: []string{
			"POSTGRES_PASSWORD=password",
			"POSTGRES_USER=postgres",
			"POSTGRES_DB=dbname",
		},
	}, func(config *docker.HostConfig) {
		config.AutoRemove = true
		config.RestartPolicy = docker.RestartPolicy{Name: "no"}
	})
	if err != nil {
		logger.Fatal("Could not start resource", zap.Error(err))
		return nil, "", nil, err
	}

	hostAndPort := resource.GetHostPort("5432/tcp")
	databaseURL := fmt.Sprintf("postgres://postgres:password@%s/dbname?sslmode=disable", hostAndPort)
	logger.Info("Launching Postgres", zap.String("url", databaseURL))

	// Wait for the database to be ready
	pool.MaxWait = 120 * time.Second
	retryCount := 0
	if err = pool.Retry(func() error {
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

		err = pool.Purge(resource)
		if err != nil {
			logger.Error("Failed to purge resource", zap.Error(err))
		} else {
			logger.Info("Successfully purged resource")
		}
	}

	return dbPool, databaseURL, cleanup, nil
}

func setupPostgresWithMigrations(pool *dockertest.Pool, logger *zap.Logger, sourceURL string) (*pgxpool.Pool, string, func(), error) {
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
