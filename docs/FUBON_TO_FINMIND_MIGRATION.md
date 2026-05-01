# FUBON_API_KEY to FINMIND_API_KEY Migration Guide

## Overview

The HybridProvider has migrated its primary data source from **Fubon** to **FinMind**. This guide helps you update your configuration.

## What Changed

- **Before**: HybridProvider used `FUBON_API_KEY` as the primary data source
- **After**: HybridProvider uses `FINMIND_API_KEY` as the primary data source
- **Fugle** remains as the secondary fallback source
- **TWSE OpenAPI** remains as the final fallback

## Migration Steps

### 1. Obtain FinMind API Key

1. Visit [FinMind Trade](https://finmindtrade.com/)
2. Register for an account
3. Generate an API key from your dashboard

### 2. Update Environment Variables

#### Option A: Update .env file

```bash
# Remove or comment out old key
# FUBON_API_KEY=your_old_key

# Add new key
FINMIND_API_KEY=your_finmind_key
```

#### Option B: Update shell environment

```bash
export FINMIND_API_KEY="your_finmind_key"
unset FUBON_API_KEY  # optional: clean up old variable
```

### 3. Verify Configuration

Run the test-hybrid command to verify:

```bash
go run ./cmd/experimental/test-hybrid
```

Expected output:
```
🔄 測試 Hybrid Provider (FinMind → Fugle → TWSE)
==================================================
✅ Provider 創建成功: hybrid-finmind
```

## Backward Compatibility

For a transitional period, the system maintains backward compatibility:

- If `FINMIND_API_KEY` is not set but `FUBON_API_KEY` is set, the system will:
  1. Use `FUBON_API_KEY` as the FinMind key
  2. Print a deprecation warning

```
[DEPRECATION] FUBON_API_KEY is deprecated for hybrid provider. Please migrate to FINMIND_API_KEY.
```

**Note**: This fallback will be removed in a future release. Please migrate as soon as possible.

## Troubleshooting

### Provider shows "hybrid-fugle" instead of "hybrid-finmind"

This means `FINMIND_API_KEY` is not set. Check:
1. The environment variable is exported
2. The `.env` file is in the project root
3. The key value is not empty

### FinMind API returns errors

- Verify your API key is valid at [FinMind Dashboard](https://finmindtrade.com/)
- Check rate limits (600 requests per minute)
- Ensure your account has sufficient quota

## Timeline

- **Now**: Both keys supported with deprecation warning
- **Next release**: Deprecation warning becomes more prominent
- **Future release**: `FUBON_API_KEY` fallback removed

## Support

For issues or questions:
- Check the [FinMind API Documentation](https://finmindtrade.com/analysis/document/)
- Review [AGENTS.md](../AGENTS.md) for data provider details
- Open an issue in the repository
