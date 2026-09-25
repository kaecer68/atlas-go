package demo

// demoStateSink 是 fixture 用的狀態容器（用 struct 指標，避免 golangci-lint 的 unused 誤報）。
type demoStateSink struct{ state int }

var demoStore = &demoStateSink{}

// DemoState 供 consumer 讀取（fixture 示範「writer → consumer」的閉環）。
func DemoState() int { return demoStore.state }

// SetWiredWriter 有 production consumer（consumer.go 呼叫）→ 不應被標記。
func SetWiredWriter(v int) {
	demoStore.state = v
}

// SetInertWriter 沒有任何呼叫端（fixture 的故意案例，由 baseline 覆蓋）。
func SetInertWriter(v int) {
	demoStore.state = v
}

// inert-ok[writer-no-consumer]: fixture 示範用豁免（理由必填）
//
// SetAnnotatedWriter 沒有任何呼叫端，但已用行內豁免註記理由 → 不應被標記。
func SetAnnotatedWriter(v int) {
	demoStore.state = v
}
