package neg

type negStateSink struct{ state int }

var negStore = &negStateSink{}

// SetDeadWriter 沒有任何呼叫端（negative fixture：檢查應該抓到）。
func SetDeadWriter(v int) {
	negStore.state = v
}

// inert-ok:
//
// SetBadAnnotationWriter 的豁免註解沒有理由（本身即違規）。
func SetBadAnnotationWriter(v int) {
	negStore.state = v
}

type Decision struct {
	Applied bool `json:"applied"`
}

// Decide 對外宣稱已生效，但 Applied 由字面值硬寫（negative fixture：檢查應該抓到）。
func Decide() Decision {
	return Decision{Applied: true}
}
