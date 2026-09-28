// Package futures 提供期貨市場的跨市場訊號特徵與影子評估（階段 1）。
//
// 內容：跨月價差結構（近月 vs 遠月）、OI 日變化、三大法人期貨淨部位、PCR 特徵，
// 以及把特徵組成影子預測並對「隔一交易日報酬符號」量測命中率的透明規則。
//
// 邊界（重要）：
//   - 本套件是**純函式**：不碰 DB、不打網路、不讀環境變數。
//   - **零隱藏係數**：所有門檻與權重都由呼叫端注入，套件內沒有預設值。
//   - **不得接進決策路徑**（部位規模／風控／歸因；`WeightFor` / `ApplySignal`）。
//     階段 2（#2110）且 R3 修好後才可討論接線。
//   - 影子列只寫獨立儲存（`futures_shadow_predictions`），不寫 live 校準表。
//
// 規格：docs/specs/futures-crossmarket-signal-spec.md
//
// Tier: utility（離線特徵與量測，非 runtime 決策模組）。
//
// Maturity: utility
package futures
