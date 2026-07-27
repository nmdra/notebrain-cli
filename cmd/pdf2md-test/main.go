package main

import (
	"context"
	"fmt"
	"os"

	"github.com/nmdra/notebrain-cli/v2/internal/pdf2md"
	"github.com/nmdra/notebrain-cli/v2/internal/pdfextract"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		return fmt.Errorf("usage: go run main.go <pdf-file>")
	}
	filePath := os.Args[1]

	ctx := context.Background()
	pb, err := pdfextract.NewPDFiumBackend()
	if err != nil {
		return fmt.Errorf("failed to initialize pdfium: %w", err)
	}
	defer pb.Close()

	rects, err := pb.ExtractStructured(ctx, filePath)
	if err != nil {
		return fmt.Errorf("failed to extract: %w", err)
	}

	markdown := pdf2md.Convert(rects)
	fmt.Println(markdown)
	return nil
}
