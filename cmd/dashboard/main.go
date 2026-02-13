package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/quay/build-dashboard/internal/cli"
	"github.com/quay/build-dashboard/internal/db"
	"github.com/quay/build-dashboard/internal/server"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "serve":
		cmdServe(os.Args[2:])
	case "report":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Usage: dashboard report <build|results|snapshot>\n")
			os.Exit(1)
		}
		switch os.Args[2] {
		case "build":
			cmdReportBuild(os.Args[3:])
		case "results":
			cmdReportResults(os.Args[3:])
		case "snapshot":
			cmdReportSnapshot(os.Args[3:])
		default:
			fmt.Fprintf(os.Stderr, "Unknown report subcommand: %s\n", os.Args[2])
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `Usage: dashboard <command>

Commands:
  serve                Start the HTTP server
  report build         Report a build to the dashboard
  report results       Report test results to the dashboard
  report snapshot      Parse a Konflux Snapshot and report builds
`)
}

func cmdServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", ":8080", "listen address")
	dbPath := fs.String("db", "dashboard.db", "SQLite database path")
	fs.Parse(args)

	database, err := db.Open(*dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer database.Close()

	srv := server.New(database, *addr)
	if err := srv.Run(); err != nil {
		log.Fatalf("server: %v", err)
	}
}

func cmdReportBuild(args []string) {
	fs := flag.NewFlagSet("report build", flag.ExitOnError)
	r := cli.BuildReport{}
	fs.StringVar(&r.Server, "server", "", "dashboard server URL")
	fs.StringVar(&r.Component, "component", "", "component name")
	fs.StringVar(&r.Version, "version", "", "build version")
	fs.StringVar(&r.GitSHA, "git-sha", "", "git commit SHA")
	fs.StringVar(&r.GitBranch, "git-branch", "", "git branch")
	fs.StringVar(&r.ImageURL, "image-url", "", "container image URL")
	fs.StringVar(&r.ImageDigest, "image-digest", "", "container image digest")
	fs.StringVar(&r.PipelineRun, "pipeline-run", "", "pipeline run name")
	fs.StringVar(&r.SnapshotName, "snapshot", "", "snapshot name")
	fs.Parse(args)

	if r.Server == "" || r.Component == "" || r.Version == "" || r.GitSHA == "" || r.ImageURL == "" {
		fmt.Fprintf(os.Stderr, "Required: --server, --component, --version, --git-sha, --image-url\n")
		fs.PrintDefaults()
		os.Exit(1)
	}

	if err := cli.ReportBuild(r); err != nil {
		log.Fatalf("report build: %v", err)
	}
}

func cmdReportResults(args []string) {
	fs := flag.NewFlagSet("report results", flag.ExitOnError)
	r := cli.ResultsReport{}
	fs.StringVar(&r.Server, "server", "", "dashboard server URL")
	fs.Int64Var(&r.BuildID, "build-id", 0, "build ID")
	fs.StringVar(&r.Suite, "suite", "", "test suite name")
	fs.StringVar(&r.Environment, "environment", "", "test environment")
	fs.StringVar(&r.PipelineRun, "pipeline-run", "", "pipeline run name")
	fs.Parse(args)

	r.Files = fs.Args()

	if r.Server == "" || r.BuildID == 0 || r.Suite == "" || len(r.Files) == 0 {
		fmt.Fprintf(os.Stderr, "Required: --server, --build-id, --suite, <junit-xml-files...>\n")
		fs.PrintDefaults()
		os.Exit(1)
	}

	if err := cli.ReportResults(r); err != nil {
		log.Fatalf("report results: %v", err)
	}
}

func cmdReportSnapshot(args []string) {
	fs := flag.NewFlagSet("report snapshot", flag.ExitOnError)
	r := cli.SnapshotReport{}
	fs.StringVar(&r.Server, "server", "", "dashboard server URL")
	fs.StringVar(&r.Version, "version", "", "build version")
	fs.Parse(args)

	remaining := fs.Args()
	if r.Server == "" || r.Version == "" || len(remaining) == 0 {
		fmt.Fprintf(os.Stderr, "Required: --server, --version, <snapshot-file>\n")
		fs.PrintDefaults()
		os.Exit(1)
	}
	r.File = remaining[0]

	if err := cli.ReportSnapshot(r); err != nil {
		log.Fatalf("report snapshot: %v", err)
	}
}
