package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"greenlight/internal/data"
	"greenlight/internal/jsonlog"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
)

const version = "1.0.0"

// config struct holds all the configuration settings for our application
type config struct {
	port int
	env  string
	db   struct {
		url          string
		maxOpenConns int
		maxIdleConns int
		maxIdleTime  string
	}
	limiter struct {
		rps     float64
		burst   int
		enabled bool
	}
}

// application struct holds the dependencies for our HTTP handlers, helpers, and middleware.
type application struct {
	config config
	logger *jsonlog.Logger
	models data.Models
}

func main() {
	logger := jsonlog.New(os.Stdout, jsonlog.LevelInfo)

	err := godotenv.Load()
	if err != nil {
		logger.PrintFatal(err, nil)
	}

	var cfg config

	port, err := strconv.Atoi(os.Getenv("PORT"))
	if err != nil {
		logger.PrintFatal(fmt.Errorf("invalid port %s", err), nil)
	}
	flag.IntVar(&cfg.port, "PORT", port, "API server port")

	environment := os.Getenv("ENVIRONEMNT")
	if _, ok := map[string]bool{"development": true, "staging": true, "production": true}[environment]; !ok {
		logger.PrintFatal(fmt.Errorf("invalid environment %s", environment), nil)
	}
	flag.StringVar(&cfg.env, "ENVIRONEMNT", "development", "Environment (development|staging|production)")

	postgresUrl := os.Getenv("POSTGRESQL_URL")
	if postgresUrl == "" {
		logger.PrintFatal(fmt.Errorf("POSTGRESQL_URL is not set"), nil)
	}
	flag.StringVar(&cfg.db.url, "POSTGRESQL_URL", postgresUrl, "PostgreSQL DSN")

	postgresMaxOpenConns, err := strconv.Atoi(os.Getenv("POSTGRESQL_MAX_OPEN_CONNS"))
	if err != nil {
		logger.PrintFatal(fmt.Errorf("invalid POSTGRESQL_MAX_OPEN_CONNS %s", err), nil)
	}
	flag.IntVar(&cfg.db.maxOpenConns, "POSTGRESQL_MAX_OPEN_CONNS",  postgresMaxOpenConns, "PostgreSQL max open connections")

	postgresMaxIdleConns, err := strconv.Atoi(os.Getenv("POSTGRESQL_MAX_IDLE_CONNS"))
	if err != nil {
		logger.PrintFatal(fmt.Errorf("invalid POSTGRESQL_MAX_IDLE_CONNS %s", err), nil)
	}
	flag.IntVar(&cfg.db.maxIdleConns, "POSTGRESQL_MAX_IDLE_CONNS", postgresMaxIdleConns, "PostgreSQL max idle connections")

	postgresMaxIdleTime := os.Getenv("POSTGRESQL_MAX_IDLE_TIME")
	if postgresMaxIdleTime == "" {
		logger.PrintFatal(fmt.Errorf("invalid POSTGRESQL_MAX_IDLE_TIME %s", err), nil)
	}
	flag.StringVar(&cfg.db.maxIdleTime, "POSTGRESQL_MAX_IDLE_TIME", postgresMaxIdleTime, "PostgreSQL max connection idle time")

	limiterRps, err := strconv.ParseFloat(os.Getenv("LIMITER_RPS"), 64)
	if err != nil {
		logger.PrintFatal(fmt.Errorf("invalid LIMITER_RPS %s", err), nil)
	} 
	flag.Float64Var(&cfg.limiter.rps, "LIMITER_RPS", limiterRps, "Rate limiter maximum requests per second")

	limiterBurst, err := strconv.Atoi(os.Getenv("LIMITER_BURST"))
	if err != nil {
		logger.PrintFatal(fmt.Errorf("invalid LIMITER_BURST %s", err), nil)
	}
	flag.IntVar(&cfg.limiter.burst, "LIMITER_BURST", limiterBurst, "Rate limiter maximum burst")

	limiterEnabled, err := strconv.ParseBool(os.Getenv("LIMITER_ENABLED"))
	if err != nil {
		logger.PrintFatal(fmt.Errorf("invalid LIMITER_ENABLED %s", err), nil)
	}
	flag.BoolVar(&cfg.limiter.enabled, "LIMITER_ENABLED", limiterEnabled, "Enable rate limiter")

	flag.Parse()

	fmt.Println(cfg)

	db, err := openDB(cfg)
	if err != nil {
		logger.PrintFatal(err, nil)
	}
	defer db.Close()

	logger.PrintInfo("database connection pool established", nil)

	app := &application{
		config: cfg,
		logger: logger,
		models: data.NewModels(db),
	}

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.port),
		Handler:      app.routes(),
		ErrorLog:     log.New(logger, "", 0),
		IdleTimeout:  time.Minute,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	logger.PrintInfo("Starting server", map[string]string{
		"addr": srv.Addr,
		"env":  cfg.env,
	})

	err = srv.ListenAndServe()
	logger.PrintFatal(err, nil)
}

func openDB(cfg config) (*sql.DB, error) {
	db, err := sql.Open("pgx", cfg.db.url)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(cfg.db.maxOpenConns)
	db.SetMaxIdleConns(cfg.db.maxIdleConns)

	duration, err := time.ParseDuration(cfg.db.maxIdleTime)
	if err != nil {
		return nil, err
	}

	db.SetConnMaxIdleTime(duration)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = db.PingContext(ctx)
	if err != nil {
		return nil, err
	}

	return db, nil
}
