package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kaecer68/atlas-go/internal/llm"
	llmAdapters "github.com/kaecer68/atlas-go/internal/llm/adapters"
	"github.com/kaecer68/atlas-go/internal/llm/capabilities"
	"github.com/kaecer68/atlas-go/internal/llm/clients"
	"github.com/kaecer68/atlas-go/internal/llm/schemas"
)

func main() {
	jsonOut := flag.Bool("json", false, "CI-friendly JSON output")
	globPattern := flag.String("glob", "configs/prompts/*.txt", "Glob pattern for prompt files")
	flag.Parse()

	files, err := filepath.Glob(*globPattern)
	if err != nil {
		fmt.Fprintf(os.Stderr, "glob error: %v\n", err)
		os.Exit(2)
	}
	if len(files) == 0 {
		fmt.Println("no prompt files found")
		os.Exit(0)
	}

	router := buildRouter()
	if router == nil {
		fmt.Fprintln(os.Stderr, "WARNING: no LLM API keys set — skipping lint (set LLM_DEEPSEEK_API_KEY and/or LLM_MINIMAX_API_KEY)")
		os.Exit(0)
	}

	handler := capabilities.NewPromptLintHandler(router)
	ctx := context.Background()

	var totalIssues int
	allClean := true

	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read error %s: %v\n", file, err)
			os.Exit(2)
		}

		resp, err := handler.Handle(ctx, schemas.PromptLintInput{
			PromptContent: string(content),
			PromptPath:    file,
			DataClass:     llm.DataClassNonRegulated,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "lint error for %s: %v\n", file, err)
			os.Exit(2)
		}

		if !resp.Pass {
			allClean = false
		}
		totalIssues += len(resp.Issues)

		if *jsonOut {
			// Print as compact single-line JSON per file for CI consumption.
			printJSON(file, resp)
		} else {
			printHuman(file, resp)
		}
	}

	if *jsonOut {
		// Print summary as last JSON line.
		fmt.Printf(`{"files":%d,"total_issues":%d,"clean":%v}`+"\n", len(files), totalIssues, allClean)
	} else {
		fmt.Printf("\n%d file(s) scanned, %d issue(s) found\n", len(files), totalIssues)
		if allClean {
			fmt.Println("all prompts pass lint")
		}
	}

	if totalIssues > 0 {
		os.Exit(1)
	}
}

// buildRouter creates an LLM Router wired with DeepSeek and MiniMax
// adapters. Returns nil when neither API key is available.
func buildRouter() llm.Router {
	deepseekKey := os.Getenv("LLM_DEEPSEEK_API_KEY")
	minimaxKey := os.Getenv("LLM_MINIMAX_API_KEY")

	if deepseekKey == "" && minimaxKey == "" {
		return nil
	}

	router := llm.NewDefaultRouter()

	if deepseekKey != "" {
		base := clients.NewBaseClient(llm.ProviderDeepSeek, clients.BaseClientConfig{})
		dc := clients.NewDeepSeekClient(deepseekKey, base)
		adapter := llmAdapters.NewDeepSeekAdapter(dc, "deepseek-v4-pro")
		_ = router.Register(adapter)
	}

	if minimaxKey != "" {
		base := clients.NewBaseClient(llm.ProviderMiniMax, clients.BaseClientConfig{})
		mc := clients.NewMiniMaxClient(minimaxKey, base)
		adapter := llmAdapters.NewMiniMaxAdapter(mc)
		_ = router.Register(adapter)
	}

	return router
}

func printJSON(file string, resp schemas.PromptLintResponse) {
	issues := resp.Issues
	if issues == nil {
		issues = []schemas.LintIssue{}
	}
	// Emit a compact single-line JSON object.
	fmt.Printf(`{"file":%q,"pass":%v,"issues":[`, file, resp.Pass)
	for i, iss := range issues {
		if i > 0 {
			fmt.Print(",")
		}
		fmt.Printf(`{"line":%d,"severity":%q,"message":%q}`, iss.Line, iss.Severity, iss.Message)
	}
	fmt.Println("]}")
}

func printHuman(file string, resp schemas.PromptLintResponse) {
	if resp.Pass {
		fmt.Printf("PASS  %s\n", file)
		return
	}
	fmt.Printf("FAIL  %s (%d issue(s))\n", file, len(resp.Issues))
	for _, iss := range resp.Issues {
		fmt.Printf("  [%s] line %d: %s\n", iss.Severity, iss.Line, iss.Message)
	}
}
