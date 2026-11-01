// openshin is the local layered-memory daemon for agents.
//
// Subcommands:
//
//	serve            run the ingest + read HTTP API
//	tail <transcript> ingest new lines from a Claude Code transcript JSONL file
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/shahab5191/openshin/internal/api"
	"github.com/shahab5191/openshin/internal/db"
	"github.com/shahab5191/openshin/internal/ingest"
	"github.com/shahab5191/openshin/internal/llm"
	"github.com/shahab5191/openshin/internal/memory"
	"github.com/shahab5191/openshin/internal/store"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "serve":
		err = runServe(os.Args[2:])
	case "tail":
		err = runTail(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		log.Fatal(err)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: openshin <serve|tail> [flags]")
}

func dbPath() string {
	if p := os.Getenv("OPENSHIN_DB_PATH"); p != "" {
		return p
	}
	return "./openshin.db"
}

func openStore() (*store.Conversations, error) {
	d, err := db.Connect(dbPath())
	if err != nil {
		return nil, err
	}
	return store.NewConversations(d), nil
}

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", ":8080", "listen address")
	if err := fs.Parse(args); err != nil {
		return err
	}

	s, err := openStore()
	if err != nil {
		return err
	}

	// The short-term context assembly needs a summarizer. It is optional: without
	// a GEMINI_API_KEY the context endpoint reports not-configured and ingest
	// still works.
	var ctx api.ContextAssembler
	if cfg, err := llm.GeminiConfigFromEnv(); err == nil {
		provider, err := llm.NewGemini(context.Background(), cfg)
		if err != nil {
			return err
		}
		ctx = memory.NewShortTerm(s, provider)
		log.Printf("short-term summarizer: %s", provider.Name())
	} else {
		log.Printf("no GEMINI_API_KEY (%v); context endpoint disabled", err)
	}

	server := api.NewServer(s, ctx)
	log.Printf("openshin: serving ingest API on %s", *addr)
	return server.ListenAndServe(*addr)
}

func runTail(args []string) error {
	fs := flag.NewFlagSet("tail", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("tail: expected exactly one transcript path")
	}

	s, err := openStore()
	if err != nil {
		return err
	}

	res, err := ingest.NewTailer(s).IngestFile(context.Background(), fs.Arg(0))
	if err != nil {
		return err
	}
	log.Printf("tail: scanned %d, ingested %d, skipped %d", res.Scanned, res.Ingested, res.Skipped)
	return nil
}
