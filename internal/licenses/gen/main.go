// Command gen writes THIRD_PARTY_LICENSES.txt at the repository root (run from the root:
// go run ./internal/licenses/gen).
package main

import (
	"fmt"
	"os"

	"github.com/Mango-Juice/agent-leeway/internal/licenses"
)

func main() {
	text, err := licenses.Generate(".")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(licenses.FileName, []byte(text), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote", licenses.FileName)
}
