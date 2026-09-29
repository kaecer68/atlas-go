package subscription

// #2109：`users.db` 必須落在**持久資料目錄**（容器 bind mount 的 <workDir>/data 內），
// 不得落在 workDir 根 —— 那是容器的可寫層，`docker compose up -d`（換 image）即遺失。

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDefaultDataDir_UnderBindMountedData 釘住慣例路徑本身。
func TestDefaultDataDir_UnderBindMountedData(t *testing.T) {
	cases := []struct {
		workDir string
		want    string
	}{
		{"/app", filepath.Join("/app", "data", "state")}, // 生產容器（CWD=/app、bind mount ./data → /app/data）
		{filepath.Join("x", "y"), filepath.Join("x", "y", "data", "state")},
	}
	for _, c := range cases {
		if got := DefaultDataDir(c.workDir); got != c.want {
			t.Errorf("DefaultDataDir(%q) = %q, want %q", c.workDir, got, c.want)
		}
		if got := DefaultDataDir(c.workDir); got == c.workDir {
			t.Errorf("DefaultDataDir(%q) must not be the workdir root (container writable layer)", c.workDir)
		}
	}
}

// TestNewStore_WritesIntoPersistentDirNotWorkdirRoot 是本票的行為釘子：
// 給 workDir ⇒ 檔案落在 <workDir>/data/state/users.db，且 workDir 根**不得**出現 users.db。
//
// 突變（把 NewStore 改回 `filepath.Join(workDir, "users.db")` 的形狀）⇒ 兩條斷言都紅。
func TestNewStore_WritesIntoPersistentDirNotWorkdirRoot(t *testing.T) {
	workDir := t.TempDir()

	store, err := NewStore(workDir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer func() { _ = store.Close() }()

	persistent := filepath.Join(workDir, "data", "state", "users.db")
	if _, err := os.Stat(persistent); err != nil {
		t.Fatalf("users.db must live in the persistent dir %s: %v", persistent, err)
	}
	if _, err := os.Stat(filepath.Join(workDir, "users.db")); err == nil {
		t.Fatalf("#2109: users.db must NOT be created at the workdir root (%s) — that is the container writable layer",
			filepath.Join(workDir, "users.db"))
	}

	// 路徑對不夠，還要真的可用（避免「只是搬了檔但 store 壞掉」）。
	if _, err := store.Register("persistent@example.test", "hash"); err != nil {
		t.Fatalf("Register: %v", err)
	}
	u, err := store.GetByEmail("persistent@example.test")
	if err != nil || u == nil {
		t.Fatalf("GetByEmail after Register = (%v, %v), want the row", u, err)
	}

	// 重新開啟同一 workDir ⇒ 看到同一份資料（證明是持久位置，不是每次新開）。
	store2, err := NewStore(workDir)
	if err != nil {
		t.Fatalf("reopen NewStore: %v", err)
	}
	defer func() { _ = store2.Close() }()
	if u2, err := store2.GetByEmail("persistent@example.test"); err != nil || u2 == nil {
		t.Fatalf("reopen lost the row: (%v, %v)", u2, err)
	}
}
