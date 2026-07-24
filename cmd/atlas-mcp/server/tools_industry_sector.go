package server

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kaecer68/atlas-go/internal/industry"
)

// tools_industry_sector.go — FU-7 Phase F: canonical sector resource MCP tools.
//
// Unlike most MCP tools in this package, these handlers do NOT proxy a REST API
// endpoint. They import internal/industry directly and compute sector metadata
// from the canonical enum (AllSectors, ClassifyBySymbol, DisplayZH). The sector
// data lives in constants, not in a database, so a REST round-trip would add
// latency without benefit.

// --- List Sectors ----------------------------------------------------------

type SectorInfo struct {
	ID          string   `json:"id"           jsonschema:"Canonical snake_case sector identifier, e.g. semiconductor"`
	DisplayZH   string   `json:"display_zh"   jsonschema:"Traditional-Chinese display label, e.g. 半導體"`
	StockSymbol []string `json:"stock_symbols,omitempty" jsonschema:"Representative stock symbols for this sector"`
}

type SectorListInput struct{}

type SectorListOutput struct {
	Sectors []SectorInfo `json:"sectors"`
}

func (s *server) handleSectorList(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, SectorListOutput, error) {
	var out SectorListOutput
	if err := s.withAudit(ctx, "industry_sector_list", nil, func() error {
		all := industry.AllSectors()
		repr := industry.DefaultRepresentativeStocks()
		out.Sectors = make([]SectorInfo, 0, len(all))
		for _, id := range all {
			syms := repr[id]
			if syms == nil {
				syms = []string{}
			}
			out.Sectors = append(out.Sectors, SectorInfo{
				ID:          string(id),
				DisplayZH:   industry.DisplayZH(id),
				StockSymbol: syms,
			})
		}
		return nil
	}); err != nil {
		return nil, SectorListOutput{}, err
	}
	return nil, out, nil
}

// --- Lookup Sector ---------------------------------------------------------

type SectorLookupInput struct {
	Symbol string `json:"symbol,omitempty" jsonschema:"Taiwan stock symbol without .TW suffix, e.g. 2330"`
	Sector string `json:"sector,omitempty" jsonschema:"Sector canonical ID, full Chinese label, or legacy alias, e.g. semiconductor or 半導體"`
}

type SectorLookupOutput struct {
	Found   bool        `json:"found"`
	Sector  *SectorInfo `json:"sector,omitempty"`
	Warning string      `json:"warning,omitempty"`
}

func (s *server) handleSectorLookup(ctx context.Context, _ *mcp.CallToolRequest, in SectorLookupInput) (*mcp.CallToolResult, SectorLookupOutput, error) {
	var out SectorLookupOutput
	if err := s.withAudit(ctx, "industry_sector_lookup", []string{in.Symbol, in.Sector}, func() error {
		// At least one parameter required
		if in.Symbol == "" && in.Sector == "" {
			out.Found = false
			out.Warning = "Provide at least one of: symbol (stock symbol) or sector (canonical ID / Chinese label)"
			return nil
		}

		var secID industry.SectorID

		if in.Symbol != "" {
			secID = industry.ClassifyBySymbol(in.Symbol)
			if secID == "" {
				out.Found = false
				out.Warning = fmt.Sprintf("Symbol %q not found in representative stocks. Use industry_sector_list to see all sectors.", in.Symbol)
				return nil
			}
		} else {
			var ok bool
			secID, ok = industry.SectorIDFromString(in.Sector)
			if !ok {
				out.Found = false
				out.Warning = fmt.Sprintf("Sector %q not recognized. Try a canonical ID (e.g. semiconductor), full Chinese label (半導體), or use industry_sector_list to see all.", in.Sector)
				return nil
			}
		}

		syms := industry.DefaultRepresentativeStocks()[secID]
		if syms == nil {
			syms = []string{}
		}

		out.Found = true
		out.Sector = &SectorInfo{
			ID:          string(secID),
			DisplayZH:   industry.DisplayZH(secID),
			StockSymbol: syms,
		}
		return nil
	}); err != nil {
		return nil, SectorLookupOutput{}, err
	}
	return nil, out, nil
}

// --- Registration ----------------------------------------------------------

func registerSectorTools(mcpSrv *mcp.Server, s *server) {
	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "industry_sector_list",
		Description: autoDescOr("industry_sector_list", "List all Taiwan-sector canonical identifiers with their Traditional-Chinese labels and representative stock symbols. Returns the full 20-sector taxonomy used by atlas-go. No input parameters required. HTTP: GET /api/industry/sectors. Alternative: industry_sector_lookup, sector_allocation_plan."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleSectorList)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "industry_sector_lookup",
		Description: autoDescOr("industry_sector_lookup", "Look up a Taiwan sector by stock symbol (e.g. 2330) or sector name/alias (canonical ID, full Chinese label, or legacy alias like 半導體, 金融, semiconductor). Returns the canonical identifier, Chinese display label, and representative stock symbols. Provide at least one of: symbol, sector. HTTP: GET /api/industry/sector-lookup?symbol=2330 or ?sector=semiconductor. Alternative: industry_sector_list, sector_allocation_plan."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleSectorLookup)
}
