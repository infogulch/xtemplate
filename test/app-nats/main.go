// Test application for xtemplate with NATS Object Store backend
//
// This app demonstrates NATS Object Store backend testing by:
// 1. Creating NATS provider using WithNats() helper
// 2. Loading Flags and Directories from JSON config
// 3. Starting xtemplate server
// 4. Uploading templates to the Object Store (triggering hot reload)
// 5. Keeping the server running for testing
//
// Run with: go run ./test/app-nats --config-file config-nats-test.json
package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/infogulch/xtemplate"
	"github.com/infogulch/xtemplate/app"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/ncruces/go-sqlite3"
	"github.com/spf13/afero"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
)

func main() {
	go func() {
		if err := sqlite3.Initialize(); err != nil {
			panic(err)
		}
	}()

	ctx := context.Background()

	// Configure embedded NATS server
	// Data will be stored in ./nats-test-data (relative to working directory)
	serverOpts := &server.Options{
		JetStream: true,
		StoreDir:  "./nats-test-data",
		Host:      "0.0.0.0",
		Port:      4222,
	}

	// Configure NATS Object Store backend
	backendConfig := &xtemplate.NatsBackendConfig{
		Bucket:      "xtemplate-test-templates",
		Prefix:      "templates/",
		EnableWatch: true,
	}

	// Start xtemplate server
	// Flags and Directories are loaded from config-nats-test.json
	// NATS provider is added using WithNats() helper
	log.Println("==> Starting xtemplate server with NATS backend")
	go app.Main(
		xtemplate.WithNats("nats", serverOpts, nil, nil, backendConfig),
	)

	// Wait for server to be ready (poll until it responds)
	log.Println("==> Waiting for server to be ready...")
	time.Sleep(500 * time.Millisecond) // Give server time to start listening
	waitForServerReady("http://localhost:8080")
	log.Println("==> Server is ready")

	// Upload templates to NATS Object Store
	// This will trigger a hot reload, loading the templates into the server
	log.Println("==> Uploading templates to NATS Object Store...")
	if err := uploadTemplates(ctx); err != nil {
		log.Fatalf("Failed to upload templates: %v", err)
	}
	log.Println("==> Templates uploaded successfully")

	// Signal that everything is ready for testing
	log.Println("==> Ready for testing")

	// Keep the main goroutine alive
	select {}
}

// waitForServerReady polls the server URL until it responds successfully
// This blocks indefinitely until the server is ready, which is important for step debugging
func waitForServerReady(url string) {
	client := &http.Client{
		Timeout: 1 * time.Second,
	}

	for {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			// Server is responding
			return
		}
		if resp != nil {
			resp.Body.Close()
		}
		// Wait a bit before retrying
		time.Sleep(100 * time.Millisecond)
	}
}

func uploadTemplates(ctx context.Context) error {
	// Connect to the NATS server (started by the provider)
	// Retry for up to 10 seconds to allow the NATS server to start
	var nc *nats.Conn
	var err error
	for i := 0; i < 100; i++ {
		nc, err = nats.Connect("nats://localhost:4222")
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		return fmt.Errorf("failed to connect to NATS after retries: %w", err)
	}
	defer nc.Close()

	// Create JetStream context
	js, err := jetstream.New(nc)
	if err != nil {
		return fmt.Errorf("failed to create JetStream context: %w", err)
	}

	// Get the Object Store
	store, err := js.ObjectStore(ctx, "xtemplate-test-templates")
	if err != nil {
		return fmt.Errorf("failed to get object store: %w", err)
	}

	// Upload all files from templates directory
	// Templates are in ./templates (relative to working directory)
	templatesDir := "./templates"
	fs := afero.NewOsFs()

	return afero.Walk(fs, templatesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Read file content
		content, err := afero.ReadFile(fs, path)
		if err != nil {
			return fmt.Errorf("failed to read file %s: %w", path, err)
		}

		// Calculate relative path
		relPath, err := filepath.Rel(templatesDir, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path for %s: %w", path, err)
		}

		// Convert to forward slashes for consistency
		relPath = filepath.ToSlash(relPath)

		// Add templates/ prefix to match backend configuration
		objectName := "templates/" + relPath

		// Upload to Object Store
		_, err = store.Put(ctx, jetstream.ObjectMeta{
			Name:        objectName,
			Description: fmt.Sprintf("Uploaded from %s", path),
		}, bytes.NewReader(content))
		if err != nil {
			return fmt.Errorf("failed to upload %s: %w", objectName, err)
		}

		log.Printf("Uploaded: %s", objectName)
		return nil
	})
}
