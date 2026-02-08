package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/exedev/news-app/internal/db"
	"github.com/exedev/news-app/internal/jobrunner"
	"github.com/exedev/news-app/internal/web"
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
		case "cleanup":
			return cleanupCmd(os.Args[2:])
		case "troubleshoot":
			return troubleshootCmd(os.Args[2:])
		case "process-articles":
			return processArticlesCmd(os.Args[2:])
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
  (default)              Start the web server
  run-job <id>           Execute a news job by ID
  cleanup                Clean up old Shelley conversations
  troubleshoot           Diagnose failed job runs
  process-articles       Process articles from JSON file
  help                   Show this help message

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

	server, err := web.New("db.sqlite3", hostname)
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

	// Set up context with signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Run job in goroutine
	errChan := make(chan error, 1)
	runner := jobrunner.NewRunner(dbConn, config)
	go func() {
		errChan <- runner.Run(ctx, jobID)
	}()

	// Wait for completion or signal
	select {
	case err := <-errChan:
		return err
	case sig := <-sigChan:
		fmt.Fprintf(os.Stderr, "\nReceived signal %v, shutting down gracefully...\n", sig)
		cancel() // Cancel context to stop job gracefully
		
		// Wait up to 10 seconds for graceful shutdown
		select {
		case err := <-errChan:
			return err
		case <-time.After(10 * time.Second):
			return fmt.Errorf("shutdown timeout")
		}
	}
}

func cleanupCmd(args []string) error {
	fs := flag.NewFlagSet("cleanup", flag.ExitOnError)
	maxAge := fs.Int("max-age", 48, "max age in hours for conversations to keep")
	dryRun := fs.Bool("dry-run", false, "show what would be deleted without deleting")
	fs.Parse(args)

	cfg := jobrunner.DefaultCleanupConfig()
	cfg.MaxAgeHours = *maxAge
	cfg.DryRun = *dryRun

	result, err := jobrunner.Cleanup(context.Background(), cfg)
	if err != nil {
		return err
	}

	fmt.Printf("Cleanup complete: found %d, deleted %d, failed %d\n",
		result.Found, result.Deleted, result.Failed)
	return nil
}

func troubleshootCmd(args []string) error {
	fs := flag.NewFlagSet("troubleshoot", flag.ExitOnError)
	lookback := fs.Int("lookback", 24, "hours to look back for problems")
	dryRun := fs.Bool("dry-run", false, "show problems without creating conversation")
	fs.Parse(args)

	cfg := jobrunner.DefaultTroubleshootConfig()
	cfg.Lookback = time.Duration(*lookback) * time.Hour
	cfg.DryRun = *dryRun

	result, err := jobrunner.Troubleshoot(context.Background(), cfg)
	if err != nil {
		return err
	}

	if result.ProblemsFound == 0 {
		fmt.Println("No problems found.")
	} else if result.ConversationID != "" {
		fmt.Printf("Found %d problems. Conversation: %s\n", result.ProblemsFound, result.ConversationID)
	} else {
		fmt.Printf("Found %d problems (dry run).\n", result.ProblemsFound)
	}
	return nil
}

func processArticlesCmd(args []string) error {
	fs := flag.NewFlagSet("process-articles", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: news-app process-articles <job_id> <articles.json>")
		fmt.Fprintln(os.Stderr, "\nProcess articles from a JSON file and save them to the database.")
	}
	fs.Parse(args)

	if fs.NArg() < 2 {
		fs.Usage()
		return fmt.Errorf("missing required arguments")
	}

	jobID, err := strconv.ParseInt(fs.Arg(0), 10, 64)
	if err != nil {
		return fmt.Errorf("invalid job ID: %w", err)
	}

	jsonPath := fs.Arg(1)

	// Read and parse JSON
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	var articles []jobrunner.ArticleInfo
	if err := json.Unmarshal(data, &articles); err != nil {
		return fmt.Errorf("parse JSON: %w", err)
	}

	fmt.Printf("Found %d articles\n", len(articles))

	// Open database
	config := jobrunner.DefaultConfig()
	dbConn, err := db.Open(config.DBPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer dbConn.Close()

	// Process articles
	runner := jobrunner.NewRunner(dbConn, config)
	saved, dups, err := runner.ProcessArticles(context.Background(), jobID, articles)
	if err != nil {
		return fmt.Errorf("process articles: %w", err)
	}

	fmt.Printf("Saved: %d, Duplicates: %d\n", saved, dups)
	return nil
}
