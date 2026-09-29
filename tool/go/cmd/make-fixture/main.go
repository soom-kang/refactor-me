// make-fixture creates a synthetic Git repository without calling a provider.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/soom-kang/refactor-me/tool/go/internal/fixture"
)

func main() {
	for _, a := range os.Args[1:] {
		if a == "--help" || a == "-h" {
			fmt.Println("Usage: make-fixture [dest] [--lang go|js] [--multi]\nDefault: Go fixture in a new temporary directory. Existing destinations are refused.\nGo tests intentionally fail TestKnownBaseline; vet and build pass. JS requires Node for validation.")
			return
		}
	}
	o, err := fixture.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	dest, err := fixture.Create(ctx, o)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fixture failed: %v\ncreated directory: %s\n", err, dest)
		os.Exit(2)
	}
	fmt.Println(dest)
}
