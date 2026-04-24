// @title           Subscriptions Service API
// @version         1.0
// @description     REST service for managing user online subscriptions
// @BasePath        /
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/ponchik327/subscriptions-service/internal/config"
	"github.com/ponchik327/subscriptions-service/internal/handler"
	"github.com/ponchik327/subscriptions-service/internal/logger"
	"github.com/ponchik327/subscriptions-service/internal/metrics"
	"github.com/ponchik327/subscriptions-service/internal/repository"
	"github.com/ponchik327/subscriptions-service/internal/service"
	"github.com/ponchik327/subscriptions-service/internal/telemetry"

	_ "github.com/ponchik327/subscriptions-service/docs"
)

func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("load config", slog.String("err", err.Error()))
		return err
	}

	log := logger.New(cfg.Log.Level)

	if err := runMigrations(cfg.Postgres.DSN, cfg.Postgres.MigrationsPath, log); err != nil {
		log.Error("migrations failed", slog.String("err", err.Error()))
		return err
	}

	shutdownTracing, err := telemetry.Setup(context.Background(), cfg.Telemetry.ServiceName, cfg.Telemetry.OTLPEndpoint)
	if err != nil {
		log.Error("init telemetry", slog.String("err", err.Error()))
		return err
	}

	pool, err := newPool(context.Background(), cfg, log)
	if err != nil {
		log.Error("connect to postgres", slog.String("err", err.Error()))
		return err
	}
	defer pool.Close()

	if err := prometheus.Register(metrics.NewPoolCollector(pool)); err != nil {
		log.Error("register pool collector", slog.String("err", err.Error()))
		return err
	}

	repo := repository.New(pool)
	svc := service.New(repo, log)
	router := handler.NewRouter(svc, pool, log)

	addr := fmt.Sprintf("%s:%d", cfg.HTTP.Host, cfg.HTTP.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
		IdleTimeout:  cfg.HTTP.ReadTimeout * 3,
	}

	log.Info("starting server", slog.String("addr", addr))

	done := make(chan struct{})
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		log.Info("shutting down server")

		ctx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Error("shutdown error", slog.String("err", err.Error()))
		}

		telCtx, telCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer telCancel()
		if err := shutdownTracing(telCtx); err != nil {
			log.Error("telemetry shutdown error", slog.String("err", err.Error()))
		}

		close(done)
	}()

	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("server error", slog.String("err", err.Error()))
		return err
	}
	<-done
	log.Info("server stopped")
	return nil
}

func runMigrations(dsn, migrationsPath string, log *slog.Logger) error {
	m, err := migrate.New(migrationsPath, dsn)
	if err != nil {
		return fmt.Errorf("create migrator: %w", err)
	}
	defer func() {
		srcErr, dbErr := m.Close()
		if srcErr != nil {
			log.Error("close migrator source", slog.String("err", srcErr.Error()))
		}
		if dbErr != nil {
			log.Error("close migrator db", slog.String("err", dbErr.Error()))
		}
	}()

	if err := m.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			log.Info("migrations: no change")
			return nil
		}
		return fmt.Errorf("run migrations: %w", err)
	}
	log.Info("migrations applied")
	return nil
}

func newPool(ctx context.Context, cfg *config.Config, log *slog.Logger) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.Postgres.DSN)
	if err != nil {
		return nil, err
	}
	poolCfg.MaxConns = cfg.Postgres.MaxConns
	poolCfg.MaxConnLifetime = 30 * time.Minute
	poolCfg.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, err
	}

	pingCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, err
	}

	log.Info("connected to postgres")
	return pool, nil
}
