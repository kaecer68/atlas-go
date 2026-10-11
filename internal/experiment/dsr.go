package experiment

import (
	"fmt"
	"math"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eval"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// ─── Deflated Sharpe Ratio（Bailey & López de Prado 2014）────────────────
//
// 對 K 次試驗選出的最優策略做多重檢定校正，並容忍非正態（偏態/峰度）。
// 語義遵循 mlfinlab 實作：SR 年化、T 為報酬筆數、峰度為 Pearson（常態=3）。
//
//   - K<2：無選擇偏差可校正（K=1 時 SR₀ 含 Φ⁻¹(0)=-∞，DSR 恆為 1），回 NaN，
//     呼叫端視為「證據不足 skip」，不是拒絕。
//   - T<3、變異數為負、分母非正：一律 NaN。

// eulerGamma 是 Euler-Mascheroni 常數（SR₀ 公式權重）。
const eulerGamma = 0.5772156649015329

// DeflatedSharpeThreshold 是 promote 的 DSR 門檻（0.95 = 5% 水準下通過
// 多重檢定）。刻意用常數而非 ParameterMetadata：DSR 是統計判據而非
// 營運參數，不進 param-table/parameters.json 連動。
const DeflatedSharpeThreshold = 0.95

// SampleSkew 回傳樣本偏態（三階動差 / 二階動差^1.5）。n<3 回 NaN。
func SampleSkew(data []float64) float64 {
	n := len(data)
	if n < 3 {
		return math.NaN()
	}
	var sum float64
	for _, v := range data {
		sum += v
	}
	mean := sum / float64(n)
	var m2, m3 float64
	for _, v := range data {
		d := v - mean
		m2 += d * d
		m3 += d * d * d
	}
	m2 /= float64(n)
	m3 /= float64(n)
	if m2 <= 0 {
		return math.NaN()
	}
	return m3 / math.Pow(m2, 1.5)
}

// SampleKurtosis 回傳 Pearson 峰度（四階動差 / 二階動差²，常態=3）。
// n<4 回 NaN。
func SampleKurtosis(data []float64) float64 {
	n := len(data)
	if n < 4 {
		return math.NaN()
	}
	var sum float64
	for _, v := range data {
		sum += v
	}
	mean := sum / float64(n)
	var m2, m4 float64
	for _, v := range data {
		d := v - mean
		m2 += d * d
		m4 += d * d * d * d
	}
	m2 /= float64(n)
	m4 /= float64(n)
	if m2 <= 0 {
		return math.NaN()
	}
	return m4 / (m2 * m2)
}

// InverseNormalCDF 是標準常態分位函數 Φ⁻¹（Acklam 有理近似，|err|<1.2e-9）。
// p ∉ (0,1) 回 NaN。
func InverseNormalCDF(p float64) float64 {
	if math.IsNaN(p) || p <= 0 || p >= 1 {
		return math.NaN()
	}
	const (
		a1    = -3.969683028665376e+01
		a2    = 2.209460984245205e+02
		a3    = -2.759285104469687e+02
		a4    = 1.383577518672690e+02
		a5    = -3.066479806614716e+01
		a6    = 2.506628277459239e+00
		b1    = -5.447609879822406e+01
		b2    = 1.615858368580409e+02
		b3    = -1.556989798598866e+02
		b4    = 6.680131188771972e+01
		b5    = -1.328068155288572e+01
		c1    = -7.784894002430293e-03
		c2    = -3.223964580411365e-01
		c3    = -2.400758277161838e+00
		c4    = -2.549732539343734e+00
		c5    = 4.374664141464968e+00
		c6    = 2.938163982698783e+00
		d1    = 7.784695709041462e-03
		d2    = 3.224671290700398e-01
		d3    = 2.445134137142996e+00
		d4    = 3.754408661907416e+00
		plow  = 0.02425
		phigh = 1 - plow
	)
	var q, x float64
	switch {
	case p < plow:
		q = math.Sqrt(-2 * math.Log(p))
		x = (((((c1*q+c2)*q+c3)*q+c4)*q+c5)*q + c6) / ((((d1*q+d2)*q+d3)*q+d4)*q + 1)
	case p <= phigh:
		q = p - 0.5
		r := q * q
		x = (((((a1*r+a2)*r+a3)*r+a4)*r+a5)*r + a6) * q / (((((b1*r+b2)*r+b3)*r+b4)*r+b5)*r + 1)
	default:
		q = math.Sqrt(-2 * math.Log(1-p))
		x = -(((((c1*q+c2)*q+c3)*q+c4)*q+c5)*q + c6) / ((((d1*q+d2)*q+d3)*q+d4)*q + 1)
	}
	return x
}

// normalCDF 是標準常態累積函數 Φ。
func normalCDF(x float64) float64 {
	return 0.5 * math.Erfc(-x/math.Sqrt2)
}

// ExpectedSharpeUnderNull 回傳虛無假設下 K 次試驗的最大期望 Sharpe（SR₀）。
// sharpeVar 為 K 個 trial Sharpe 的截面變異數。K<2 或變異數為負回 NaN。
func ExpectedSharpeUnderNull(sharpeVar float64, numTrials int) float64 {
	if numTrials < 2 || sharpeVar < 0 || math.IsNaN(sharpeVar) {
		return math.NaN()
	}
	k := float64(numTrials)
	return math.Sqrt(sharpeVar) * ((1-eulerGamma)*InverseNormalCDF(1-1/k) +
		eulerGamma*InverseNormalCDF(1-1/(k*math.E)))
}

// DeflatedSharpeRatio 回傳 DSR = P(真實 Sharpe > 0 | 觀測)，已對 K 次試驗的
// 選擇偏差與非常態校正。estimatedSR 年化、horizon 為報酬筆數、峰度 Pearson。
// 證據不足（K<2、T<3、變異數為負、輸入 NaN、分母非正）回 NaN。
func DeflatedSharpeRatio(estimatedSR, sharpeVar float64, numTrials, horizon int, skew, kurtosis float64) float64 {
	if numTrials < 2 || horizon < 3 || sharpeVar < 0 ||
		math.IsNaN(estimatedSR) || math.IsNaN(sharpeVar) ||
		math.IsNaN(skew) || math.IsNaN(kurtosis) {
		return math.NaN()
	}
	sr0 := ExpectedSharpeUnderNull(sharpeVar, numTrials)
	denom := math.Sqrt(1 - skew*estimatedSR + (kurtosis-1)/4*estimatedSR*estimatedSR)
	if math.IsNaN(sr0) || math.IsNaN(denom) || denom <= 0 {
		return math.NaN()
	}
	return normalCDF((estimatedSR - sr0) * math.Sqrt(float64(horizon-1)) / denom)
}

// TrialSharpeVariance 回傳 trial Sharpe 序列的樣本變異數（n-1）。不足 2 點
// 回 ok=false（呼叫端此時 K<2，本就 skip DSR）。
func TrialSharpeVariance(sharpes []float64) (variance float64, ok bool) {
	n := len(sharpes)
	if n < 2 {
		return 0, false
	}
	var sum float64
	for _, s := range sharpes {
		sum += s
	}
	mean := sum / float64(n)
	var sq float64
	for _, s := range sharpes {
		d := s - mean
		sq += d * d
	}
	return sq / float64(n-1), true
}

// dsrVerdict 計算單一候選的 DSR  verdict。history 為同範圍歷史 trial
// Sharpe（不含當前 trial）；當前 trial Sharpe 由 candidateReturns 經
// eval.SharpeRatio 年化口徑估計。證據不足（報酬 <4 筆、歷史為空即 K<1、
// 動差退化）回 dsr=NaN，呼叫端 skip 不拒絕。trials 含當前 trial。
func dsrVerdict(candidateReturns []float64, history []float64) (dsr float64, trials int) {
	trials = len(history) + 1
	if len(candidateReturns) < 4 || len(history) < 1 {
		return math.NaN(), trials
	}
	sr := eval.SharpeRatio(candidateReturns, 0)
	all := make([]float64, 0, len(history)+1)
	all = append(all, history...)
	all = append(all, sr)
	v, ok := TrialSharpeVariance(all)
	if !ok {
		return math.NaN(), trials
	}
	skew := SampleSkew(candidateReturns)
	kurt := SampleKurtosis(candidateReturns)
	return DeflatedSharpeRatio(sr, v, len(all), len(candidateReturns), skew, kurt), len(all)
}

// deflatedSharpeGate 在 passesAcceptance 內執行 DSR 多重檢定校正。
// 回 (true, reason) 拒絕；(false, "") 通過或 skip——每次 skip 都 debug-log
// （observability 紀律：任何 skip 必須可觀測）。
//
// K 範圍 = 同 (Brief.TargetAgentID, Brief.MutationType) 的歷史 trial。
// K<2、無 loader、載入失敗、證據不足一律 fail-open skip：單一 trial 沒有
// 選擇偏差可校正（SR₀ 含 Φ⁻¹(0)=-∞，DSR 恆為 1），舊實驗也不因此全擋。
func (j *Judge) deflatedSharpeGate(result domain.PromptExperimentResult) (bool, string) {
	expID := result.Experiment.ID
	returns := result.CandidateReturns
	if len(returns) < 4 {
		logging.Debug("judge", "dsr_skip",
			"reason", "insufficient returns", "experiment_id", expID)
		return false, ""
	}
	if j.store == nil {
		logging.Debug("judge", "dsr_skip",
			"reason", "nil store", "experiment_id", expID)
		return false, ""
	}
	loader, ok := j.store.(ledger.TrialSharpeLoader)
	if !ok {
		logging.Debug("judge", "dsr_skip",
			"reason", "no trial sharpe loader", "experiment_id", expID)
		return false, ""
	}
	trials, err := loader.LoadTrialSharpes()
	if err != nil {
		logging.Debug("judge", "dsr_skip",
			"reason", "load error", "experiment_id", expID, "err", err)
		return false, ""
	}
	target := result.Brief.TargetAgentID
	mutation := result.Brief.MutationType
	var history []float64
	for _, ts := range trials {
		if ts.TargetAgentID == target && ts.MutationType == mutation {
			history = append(history, ts.Sharpe)
		}
	}
	dsr, k := dsrVerdict(returns, history)
	if math.IsNaN(dsr) {
		logging.Debug("judge", "dsr_skip",
			"reason", "K<2 no selection bias to correct",
			"experiment_id", expID, "trials", k)
		return false, ""
	}
	if dsr < DeflatedSharpeThreshold {
		return true, fmt.Sprintf("rejected: deflated Sharpe %.3f < %.2f (K=%d same-scope trials share the selection)",
			dsr, DeflatedSharpeThreshold, k)
	}
	return false, ""
}
