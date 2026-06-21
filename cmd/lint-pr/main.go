// Command lint-pr annotates a Git diff with code review findings via LLM
// providers (MiniMax + DeepSeek). It reads the diff from stdin, routes the
// annotation request through the capability-based LLM router, and prints
// issues (file, line, severity, message) as JSON to stdout.
//
// Usage:
//
//	git diff | go run ./cmd/lint-pr
//	git diff | go run ./cmd/lint-pr --pr-url https://github.com/example/pr/42
//	git diff | go run ./cmd/lint-pr --json
//
// Exit codes:
//
//	0 — clean (no issues or no API keys configured)
//	1 — issues found
//	2 — runtime error
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/llm"
	"github.com/kaecer68/atlas-go/internal/llm/adapters"
	"github.com/kaecer68/atlas-go/internal/llm/capabilities"
	"github.com/kaecer68/atlas-go/internal/llm/clients"
	"github.com/kaecer68/atlas-go/internal/llm/schemas"
)

func main() {
	jsonMode := flag.Bool("json", false, "output as JSON")
	prURL := flag.String("pr-url", "", "PR URL for context")
	flag.Parse()

	if *jsonMode {
		jsonFlag := true
		_ = jsonFlag // suppress unused warning; --json == --json
	}

	diffText, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lint-pr: read stdin: %v\n", err)
		os.Exit(2)
	}
	if len(diffText) == 0 {
		fmt.Fprintln(os.Stderr, "lint-pr: no diff on stdin")
		os.Exit(2)
	}

	router := buildRouter()
	if router == nil {
		fmt.Fprintln(os.Stderr, "lint-pr: no API keys configured (set LLM_DEEPSEEK_API_KEY or LLM_MINIMAX_API_KEY)")
		os.Exit(0)
	}

	handler := capabilities.NewCodeReviewAnnotationHandler(router)
	input := schemas.CodeReviewAnnotationInput{
		DiffText:  string(diffText),
		PRURL:     *prURL,
		DataClass: llm.DataClassNonRegulated,
	}

	ctx := context.Background()
	resp, err := handler.Handle(ctx, input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lint-pr: %v\n", err)
		os.Exit(2)
	}

	if *jsonMode {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if encErr := enc.Encode(resp); encErr != nil {
			fmt.Fprintf(os.Stderr, "lint-pr: encode: %v\n", encErr)
			os.Exit(2)
		}
	} else {
		for _, a := range resp.Annotations {
			fmt.Printf("%s:%d: %s: %s\n", a.File, a.Line, a.Severity, a.Message)
		}
	}

	if len(resp.Annotations) > 0 {
		os.Exit(1)
	}
}

// buildRouter creates an llm.Router with MiniMax and DeepSeek adapters
// registered (only for providers whose API key is set in the environment).
// Returns nil when no API keys are configured.
func buildRouter() llm.Router {
	var impls []llm.ProviderImpl

	if apiKey := config.GetSecret("LLM_DEEPSEEK_API_KEY"); apiKey != "" {
		dsClient := clients.NewDeepSeekClient(apiKey, nil)
		dsAdapter := adapters.NewDeepSeekAdapter(dsClient, "deepseek-v4-pro")
		impls = append(impls, dsAdapter)
	}

	if apiKey := config.GetSecret("LLM_MINIMAX_API_KEY"); apiKey != "" {
		mmClient := clients.NewMiniMaxClient(apiKey, nil)
		mmAdapter := adapters.NewMiniMaxAdapter(mmClient)
		impls = append(impls, mmAdapter)
	}

	if len(impls) == 0 {
		return nil
	}

	return llm.NewDefaultRouter(impls...)
}
