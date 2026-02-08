package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"

	"srv.exe.dev/db"
	"srv.exe.dev/jobrunner"
	"srv.exe.dev/srv"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	// Check for subcommand
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "run-job":
			return runJobCmd(os.Args[2:])
		case "help", "-h", "--help":
			printUsage()
			return nil
		}
	}

	// Default: run server
	return runServer()
}

func printUsage() {
	fmt.Println(`Usage: news-app [command]

Commands:
  (default)      Start the web server
  run-job <id>   Execute a news job by ID
  help           Show this help message

Server flags:`)
	flag.PrintDefaults()
}

func runServer() error {
	listenAddr := flag.String("listen", ":8000", "address to listen on")
	flag.Parse()

	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	server, err := srv.New("db.sqlite3", hostname)
	if err != nil {
		return fmt.Errorf("create server: %w", err)
	}

	return server.Serve(*listenAddr)
}

func runJobCmd(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: news-app run-job <job_id>")
	}

	jobID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid job ID: %w", err)
	}

	// Open database
	config := jobrunner.DefaultConfig()
	dbConn, err := db.Open(config.DBPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer dbConn.Close()

	// Run the job
	runner := jobrunner.NewRunner(dbConn, config)
	return runner.Run(context.Background(), jobID)
}
