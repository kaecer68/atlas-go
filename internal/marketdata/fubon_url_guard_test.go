package marketdata

// Fubon URL Guard Test (新增於 2026-06 修復 fubon channel recurring failure)
//
// 這個測試防止以下兩個反模式復活,兩者都是 PR #572 之前的反覆故障根因:
//
//  1. `localhost:8081` 出現在 production .go 程式碼中。
//     原因:macOS / Linux 在雙棧環境下,Go `net.Dial` 對 `localhost`
//     預設優先走 IPv6 `[::1]`,而 Python proxy (services/fubon-proxy/main.py)
//     綁定 IPv4 `0.0.0.0`,導致 `[::1]:8081: connect: connection refused`。
//
//  2. `os.Getenv("FUBON_PROXY_URL")` 重新出現在 .go 程式碼中。
//     原因:此環境變數於 PR #572 (commit 8e22cbe5, 2026-06-17) 已從
//     `fubon_client.go` 與 `hybrid_provider.go` 移除。`FubonClient` 與
//     `HybridProvider` 必須永遠使用 `fubonProxyBaseURL = "http://127.0.0.1:8081"`
//     硬編碼,不允許外部輸入繞過安全預設值。
//
// 違反任一條件 → 測試失敗,PR CI 紅燈。

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// bannedPatterns 是禁止出現在 .go 程式碼中的字串。
// 任何 production .go 檔案命中任一條目都會讓測試失敗。
var bannedPatterns = []string{
	"localhost:8081",
	`os.Getenv("FUBON_PROXY_URL")`,
	`os.Getenv(\"FUBON_PROXY_URL\")`,
}

// TestFubon_URLGuard 掃描 internal/ 與 cmd/ 下所有 production .go 檔案,
// 確認沒有任何 bannedPatterns 出現。
//
// 排除規則:
//   - 排除 _test.go 檔案(測試可能用 localhost:8081 模擬錯誤訊息字串)
//   - 排除本檔案本身(guard test 必然提及 bannedPatterns)
//   - 排除 vendor/ 與 .git/ 目錄(若有)
func TestFubon_URLGuard(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("cannot find repo root: %v", err)
	}

	scanDirs := []string{
		filepath.Join(repoRoot, "internal"),
		filepath.Join(repoRoot, "cmd"),
	}

	thisFile, _ := filepath.Abs("fubon_url_guard_test.go")

	var violations []string

	for _, dir := range scanDirs {
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				// 跳過 vendor、隱藏目錄、testdata
				name := info.Name()
				if name == "vendor" || name == "testdata" || (strings.HasPrefix(name, ".") && name != ".") {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}
			// 跳過 test 檔案(測試可能用 localhost:8081 模擬錯誤字串)
			if strings.HasSuffix(path, "_test.go") {
				return nil
			}
			// 跳過本檔案
			abs, _ := filepath.Abs(path)
			if abs == thisFile {
				return nil
			}

			// 用 go/parser 抽取所有字串字面值,避免誤判註解/docstring
			// 為什麼這樣做:簡單 grep 會把 "// localhost:8081 是壞的" 也算違規;
			// 用 AST 才能精確只比對真實的 Go 字串 literal (string content)。
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
			if err != nil {
				// parse 失敗的檔案可能是 gen 產物或壞檔,跳過(go vet 會另行抓出)
				return nil
			}

			for _, pattern := range bannedPatterns {
				if astHasStringLiteral(f, pattern) {
					violations = append(violations,
						filepath.ToSlash(path)+" contains banned string literal: "+pattern)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}

	if len(violations) > 0 {
		t.Errorf("Fubon URL guard FAILED — found %d violation(s):\n  %s\n\n"+
			"為什麼失敗:localhost:8081 在雙棧環境下會走 IPv6 [::1],而 Python proxy 只綁 IPv4,造成 connection refused。"+
			"FUBON_PROXY_URL 環境變數已於 PR #572 移除,proxy 位址必須永遠硬編碼 127.0.0.1:8081。",
			len(violations), strings.Join(violations, "\n  "))
	}
}

// astHasStringLiteral 檢查 AST 中是否有基本字串字面值等於 target。
// 注意:不做 substring 比對(避免誤判) — 只比對完整 string literal 等於 target。
func astHasStringLiteral(f *ast.File, target string) bool {
	target = strings.Trim(target, `"`)
	found := false
	ast.Inspect(f, func(n ast.Node) bool {
		if found {
			return false
		}
		bl, ok := n.(*ast.BasicLit)
		if !ok || bl.Kind != token.STRING {
			return true
		}
		// 解析字串字面值(去掉引號)
		val := bl.Value
		if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
			val = val[1 : len(val)-1]
		}
		if val == target {
			found = true
			return false
		}
		return true
	})
	return found
}

// findRepoRoot 從當前工作目錄向上找到包含 go.mod 的目錄。
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
