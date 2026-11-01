// openshin is the local layered-memory daemon for agents.
//
// Subcommands:
//
//	serve            run the ingest + read HTTP API (and the memory pipeline)
//	tail <transcript> ingest new lines from a Claude Code transcript JSONL file
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

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

func openDB() (*sql.DB, error) {
	return db.Connect(dbPath())
}

// buildProvider returns a Gemini provider if a key is configured, else nil. The
// memory pipeline (rolling summary, mid-term extraction) is disabled without it;
// ingest still works.
func buildProvider() (*llm.Gemini, error) {
	cfg, err := llm.GeminiConfigFromEnv()
	if err != nil {
		return nil, err
	}
	return llm.NewGemini(context.Background(), cfg)
}

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", ":8080", "listen address")
	promoteEvery := fs.Duration("promote-every", 5*time.Second, "mid-term promotion interval")
	if err := fs.Parse(args); err != nil {
		return err
	}

	d, err := openDB()
	if err != nil {
		return err
	}

	convStore := store.NewConversations(d)
	memStore := store.NewMemory(d)

	var ctx api.ContextAssembler
	var searcher api.Searcher
	if provider, err := buildProvider(); err == nil {
		ctx = memory.NewShortTerm(convStore, provider)
		searcher = memory.NewRetriever(memStore, provider)
		log.Printf("memory provider: %s", provider.Name())

		mid := memory.NewMidTerm(memStore, provider, provider, provider)
		long := memory.NewLongTerm(memStore, provider, provider)
		skill := memory.NewSkillMemory(memStore, provider, provider)
		go runPipeline(mid, long, skill, *promoteEvery)
		log.Printf("memory pipeline: every %s", *promoteEvery)
	} else {
		log.Printf("no GEMINI_API_KEY (%v); memory pipeline disabled, ingest still works", err)
	}

	server := api.NewServer(convStore, ctx, searcher)
	log.Printf("openshin: serving ingest API on %s", *addr)
	return server.ListenAndServe(*addr)
}

func runPipeline(mid *memory.MidTerm, long *memory.LongTerm, skill *memory.SkillMemory, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		ctx := context.Background()
		if err := mid.Run(ctx); err != nil {
			log.Printf("mid-term: %v", err)
		}
		if err := long.Run(ctx); err != nil {
			log.Printf("long-term: %v", err)
		}
		if err := skill.Run(ctx); err != nil {
			log.Printf("skill: %v", err)
		}
	}
}

func runTail(args []string) error {
	fs := flag.NewFlagSet("tail", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("tail: expected exactly one transcript path")
	}

	d, err := openDB()
	if err != nil {
		return err
	}

	res, err := ingest.NewTailer(store.NewConversations(d)).IngestFile(context.Background(), fs.Arg(0))
	if err != nil {
		return err
	}
	log.Printf("tail: scanned %d, ingested %d, skipped %d", res.Scanned, res.Ingested, res.Skipped)
	return nil
}
