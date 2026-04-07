package marketdata

import (
	"context"
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// HybridProvider 混合数据源 Provider
// 优先使用 Fugle（实时），失败时回退到 TWSE OpenAPI（免费但 rate limited）
type HybridProvider struct {
	fugleProvider *FugleProvider
	twseClient    *TWSEClient
	useTWSE       bool // 当 Fugle 失败时切换到 TWSE
}

// NewHybridProvider 创建混合 Provider
// apiKey: Fugle API key，如果为空或失败则回退到 TWSE
func NewHybridProvider(apiKey string) *HybridProvider {
	var fugleProvider *FugleProvider
	if apiKey != "" {
		fugleProvider = NewFugleProviderWithAPIKey(apiKey)
	}

	return &HybridProvider{
		fugleProvider: fugleProvider,
		twseClient:    NewTWSEClient(),
		useTWSE:       fugleProvider == nil,
	}
}

// Name 返回 Provider 名称
func (p *HybridProvider) Name() string {
	if p.useTWSE || p.fugleProvider == nil {
		return "hybrid-twse"
	}
	return "hybrid-fugle"
}

// GetQuotes 获取行情，优先 Fugle，失败时回退 TWSE
func (p *HybridProvider) GetQuotes(ctx context.Context, asOf time.Time, symbols []string) ([]domain.Quote, error) {
	// 如果已经切换到 TWSE 模式，直接使用 TWSE
	if p.useTWSE || p.fugleProvider == nil {
		return p.getQuotesFromTWSE(ctx, symbols)
	}

	// 尝试使用 Fugle
	fugleQuotes, err := p.fugleProvider.GetQuotes(ctx, asOf, symbols)
	if err != nil {
		// Fugle 失败，记录并切换到 TWSE
		fmt.Printf("[HybridProvider] Fugle failed (%v), falling back to TWSE\n", err)
		p.useTWSE = true
		return p.getQuotesFromTWSE(ctx, symbols)
	}

	// 检查 Fugle 返回的数据是否完整
	if len(fugleQuotes) == 0 || p.hasInvalidQuotes(fugleQuotes) {
		fmt.Printf("[HybridProvider] Fugle returned invalid/empty data, trying TWSE\n")
		p.useTWSE = true
		twseQuotes, twseErr := p.getQuotesFromTWSE(ctx, symbols)
		if twseErr == nil && len(twseQuotes) > 0 {
			return twseQuotes, nil
		}
		// TWSE 也失败，但至少返回 Fugle 的结果
		return fugleQuotes, err
	}

	return fugleQuotes, nil
}

// getQuotesFromTWSE 从 TWSE 获取行情
func (p *HybridProvider) getQuotesFromTWSE(ctx context.Context, symbols []string) ([]domain.Quote, error) {
	if len(symbols) == 1 {
		// 单个股票查询
		quote, err := p.twseClient.GetQuote(ctx, symbols[0])
		if err != nil {
			return nil, err
		}
		return []domain.Quote{quote}, nil
	}

	// 批量查询
	return p.twseClient.GetQuotesBySymbols(ctx, symbols)
}

// hasInvalidQuotes 检查是否有无效的行情数据（如价格为 0）
func (p *HybridProvider) hasInvalidQuotes(quotes []domain.Quote) bool {
	for _, q := range quotes {
		if q.Last == 0 && q.Open == 0 && q.High == 0 && q.Low == 0 {
			return true
		}
	}
	return false
}

// Reset 重置 Provider 状态（重新尝试 Fugle）
func (p *HybridProvider) Reset() {
	p.useTWSE = p.fugleProvider == nil
}

// UseTWSE 强制使用 TWSE（忽略 Fugle）
func (p *HybridProvider) UseTWSE() {
	p.useTWSE = true
}

// UseFugle 强制使用 Fugle（如果配置了）
func (p *HybridProvider) UseFugle() {
	p.useTWSE = false
}

// GetClient 获取底层客户端（用于直接访问）
func (p *HybridProvider) GetTWSEClient() *TWSEClient {
	return p.twseClient
}

func (p *HybridProvider) GetFugleClient() *FugleClient {
	if p.fugleProvider == nil {
		return nil
	}
	return p.fugleProvider.GetClient()
}

// IsUsingTWSE 返回当前是否使用 TWSE
func (p *HybridProvider) IsUsingTWSE() bool {
	return p.useTWSE
}
