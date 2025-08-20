package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"swagger-annotator/internal/annotation"
)

func main() {
	mode := flag.String("mode", "annotate", "Mode: annotate or check")
	flag.Parse()

	switch *mode {
	case "annotate":
		if err := annotation.Run(); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "annotate failed: %v\n", err)
			os.Exit(1)
		}
	case "check":
		err := annotation.Run()
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "check failed: %v\n", err)
			os.Exit(1)
		}
		// Extra check: if `git status` is dirty, fail
		out, _ := execCommand("git", "status", "--porcelain")
		if len(out) > 0 {
			_, _ = fmt.Fprintln(os.Stderr, "annotation check failed: uncommitted changes found")
			_, _ = fmt.Fprintln(os.Stderr, string(out))
			os.Exit(2)
		}
	default:
		_, _ = fmt.Fprintf(os.Stderr, "invalid mode: %s\n", *mode)
		os.Exit(1)
	}
}

func execCommand(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}
