package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"greenlight/internal/data"
	"log"
	"net/http"
	"os"
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
}

// application struct holds the dependencies for our HTTP handlers, helpers, and middleware.
type application struct {
	config config
	logger *log.Logger
	models data.Models
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file:", err)
	}

	var cfg config
	flag.IntVar(&cfg.port, "port", 4000, "API server port")
	flag.StringVar(&cfg.env, "env", "development", "Environment (development|staging|production)")
	flag.StringVar(&cfg.db.url, "POSTGRESQL_URL", os.Getenv("POSTGRESQL_URL"), "PostgreSQL DSN")
	flag.IntVar(&cfg.db.maxOpenConns, "POSTGRESQL_MAX_OPEN_CONNS", 25, "PostgreSQL max open connections")
	flag.IntVar(&cfg.db.maxIdleConns, "POSTGRESQL_MAX_IDLE_CONNS", 25, "PostgreSQL max idle connections")
	flag.StringVar(&cfg.db.maxIdleTime, "POSTGRESQL_MAX_IDLE_TIME", "15m", "PostgreSQL max connection idle time")
	flag.Parse()

	logger := log.New(os.Stdout, "", log.Ldate|log.Ltime)

	db, err := openDB(cfg)
	if err != nil {
		logger.Fatal(err)
	}
	defer db.Close()

	logger.Printf("database connection pool established")

	app := &application{
		config: cfg,
		logger: logger,
		models: data.NewModels(db),
	}

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.port),
		Handler:      app.routes(),
		IdleTimeout:  time.Minute,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	logger.Printf("Starting %s server on %s", cfg.env, srv.Addr)
	err = srv.ListenAndServe()
	logger.Fatal(err)
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
