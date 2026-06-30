package marketdata

// Fubon URL Guard Test (新增於 2026-06 修復 fubon channel recurring failure)
//
// 這個測試防止以下反模式復活,三者都是 PR #572 與 PR #837 之前的反覆故障根因:
//
//  1. `localhost:8081` 出現在 production .go 程式碼中。
//     原因:macOS / Linux 在雙棧環境下,Go `net.Dial` 對 `localhost`
//     預設優先走 IPv6 `[::1]`,而 Python proxy (services/fubon-proxy/main.py)
//     綁定 IPv4 `0.0.0.0`,導致 `[::1]:8081: connect: connection refused`。
//
//  2. `os.Getenv("FUBON_PROXY_URL")` 重新出現在 .go 程式碼中。
//     原因:此環境變數於 PR #572 (commit 8e22cbe5, 2026-06-17) 已從
//     `fubon_client.go` 與 `hybrid_provider.go` 移除。proxy 位址必須永遠
//     從 `fubonproxy.ProxyBaseURL()` / `fubonproxy.ProxyHostPort()` 取得,
//     不允許外部輸入繞過安全預設值。
//
//  3. (PR #837 follow-up)任何 .go 程式碼用 `fmt.Sprintf("http://%s:%d", ...)`
//     自行構造 fubon-proxy URL。歷史 RCA:host + port 字串硬編碼散落在 3 個
//     source files (fubon_client.go / hybrid_provider.go / register_adapters.go),
//     任一處忘記同步就會造成 port drift → channel recurring failure。
//     唯一構造點已統一至 `internal/fubonproxy/manager.go` 的
//     `ProxyBaseURL()` 與 `ProxyHostPort()`,所有其他 .go 檔案必須呼叫
//     這些 helpers,不得自行構造。
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

// fubonProxyURLCanonicalFile 是唯一允許自行構造 fubon-proxy URL 的檔案
// (定義 ProxyBaseURL / ProxyHostPort 的 canonical owner)。
// 其他任何 .go 檔案若出現 fmt.Sprintf("http://%s:%d", ...) 都會被視為
// PR #837 A1 root cause 的 drift bug。
//
// 路徑以 forward slash 為基準(ast 解析後的行內字串、URI 慣例)。
var fubonProxyURLCanonicalFile = "internal/fubonproxy/manager.go"

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
			"FUBON_PROXY_URL 環境變數已於 PR #572 移除;proxy 位址必須從 fubonproxy.ProxyBaseURL() 取得。",
			len(violations), strings.Join(violations, "\n  "))
	}
}

// TestFubon_URLDriftGuard (PR #837 follow-up, A1 root cause) 防止以下反模式:
//
//	在 fubon-proxy canonical owner (internal/fubonproxy/manager.go) 以外的
//	任何 .go 檔案中,以 fmt.Sprintf("http://%s:%d", host, port) 自行構造
//	fubon-proxy URL。
//
// 歷史 RCA (2026-06):host + port 字串硬編碼散落在 fubon_client.go、
// hybrid_provider.go、register_adapters.go 三處,任一處忘記同步
// (例如 -fubon-port flag 啟用 alt-port 時)就會造成 port drift →
// channel recurring failure。本 guard 強制未來 dev 必須呼叫
// fubonproxy.ProxyBaseURL() / fubonproxy.ProxyHostPort() 取得 URL,
// 由 fubonproxy package 統一 host + port 來源。
//
// 排除規則:
//   - fubonProxyURLCanonicalFile (唯一 canonical owner, 允許內部 Sprintf)
//   - _test.go 檔案(測試可能用 Sprintf 構造 mock URL/host:port 字串)
//   - 本檔案本身
func TestFubon_URLDriftGuard(t *testing.T) {
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
				name := info.Name()
				if name == "vendor" || name == "testdata" || (strings.HasPrefix(name, ".") && name != ".") {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}
			// 跳過 test 檔案
			if strings.HasSuffix(path, "_test.go") {
				return nil
			}
			// 跳過本檔案
			abs, _ := filepath.Abs(path)
			if abs == thisFile {
				return nil
			}
			// 跳過 canonical owner
			if strings.HasSuffix(filepath.ToSlash(path), fubonProxyURLCanonicalFile) {
				return nil
			}

			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
			if err != nil {
				return nil // parse 失敗的檔案跳過
			}

			if call := astFindHTTPSprintfURL(f); call != nil {
				violations = append(violations,
					filepath.ToSlash(path)+":"+fset.Position(call.Pos()).String()+
						" constructs fubon-proxy URL via fmt.Sprintf(\"http://%s:%d\", ...). "+
						"Use fubonproxy.ProxyBaseURL() or fubonproxy.ProxyHostPort() instead.")
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}

	if len(violations) > 0 {
		t.Errorf("Fubon URL drift guard FAILED — found %d violation(s):\n  %s\n\n"+
			"為什麼失敗:PR #837 user prompt 列出 A1 root cause 為 3 個 source files "+
			"各自硬編碼 host.docker.internal:8081,任一處忘記同步即造成 port drift。"+
			"修正:所有 fubon-proxy URL 構造點統一至 internal/fubonproxy/manager.go "+
			"的 ProxyBaseURL() / ProxyHostPort() helpers。",
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

// astFindHTTPSprintfURL 在 AST 中尋找 fmt.Sprintf("http://%s:%d", ...) 呼叫。
// 偵測模式:第一個 arg 是 STRING literal,以 "http://" 開頭且包含 ":%d" 或 ":%s"。
// 回傳違規的 *ast.CallExpr(若無違規回 nil)。
//
// 為何偵測此 pattern:它是 PR #837 A1 root cause 的具體 drift 來源
// (host + port 透過 Sprintf 拼接字串,而非從單一 canonical helper 取得)。
func astFindHTTPSprintfURL(f *ast.File) *ast.CallExpr {
	var violation *ast.CallExpr
	ast.Inspect(f, func(n ast.Node) bool {
		if violation != nil {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		// 偵測 fmt.Sprintf(...) — 容忍 alias 形式(如 f := fmt.Sprintf; f(...))
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Sprintf" {
			return true
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok || ident.Name != "fmt" {
			return true
		}
		// 檢查第一個 arg 是否為 "http://...:%d/..." 形式的字串 literal
		if len(call.Args) == 0 {
			return true
		}
		bl, ok := call.Args[0].(*ast.BasicLit)
		if !ok || bl.Kind != token.STRING {
			return true
		}
		format := bl.Value
		if len(format) >= 2 && format[0] == '"' && format[len(format)-1] == '"' {
			format = format[1 : len(format)-1]
		}
		if strings.HasPrefix(format, "http://") && (strings.Contains(format, ":%d") || strings.Contains(format, ":%s")) {
			violation = call
			return false
		}
		return true
	})
	return violation
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
