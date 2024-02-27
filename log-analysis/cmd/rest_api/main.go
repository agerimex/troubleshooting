package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"logs-backend/internal/data"
	"logs-backend/internal/driver"
)

type application struct {
	serviceConfig ServiceConfig
	infoLog       *log.Logger
	errorLog      *log.Logger
	environment   string
	models        data.Models
}

type ServiceConfig struct {
	port int
}

func initOpenTelemetry() {
	_, err := sender.NewTracer("LogAnalysis", "localhost")
	if err != nil {
		fmt.Println("Where is receiver of traces")
	}
}

func main() {
	var cfg ServiceConfig

	cfg.port = 8094

	infoLog := log.New(os.Stdout, "INFO\t", log.Ldate|log.Ltime|log.Lshortfile)
	errorLog := log.New(os.Stdout, "ERROR\t", log.Ldate|log.Ltime|log.Lshortfile)

	db, err := driver.Connect()

	if err != nil {
		log.Fatal(err)
	}

	// initOpenTelemetry()

	app := &application{
		serviceConfig: cfg,
		infoLog:       infoLog,
		errorLog:      errorLog,
		environment:   "",
		models:        data.New(db),
	}

	err = app.serve()
	if err != nil {
		log.Fatal(err)
	}
}

func (app *application) serve() error {

	app.infoLog.Println("API listening on port", app.serviceConfig.port)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", app.serviceConfig.port),
		Handler: app.routers(),
	}

	return srv.ListenAndServe()
}
