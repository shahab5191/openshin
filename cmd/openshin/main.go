// openshin is the local layered-memory daemon for agents. Phase 0 scaffolds the
// binary and proves the SQLite connection; the serve/mcp/context subcommands and
// the memory pipeline arrive in later phases.
package main

import (
	"log"
	"os"

	"github.com/shahab5191/openshin/internal/db"
)

func main() {
	path := os.Getenv("OPENSHIN_DB_PATH")
	if path == "" {
		path = "./openshin.db"
	}

	d, err := db.Connect(path)
	if err != nil {
		log.Fatalf("connect database: %v", err)
	}
	defer d.Close()

	log.Printf("openshin: memory database ready at %q", path)
}
