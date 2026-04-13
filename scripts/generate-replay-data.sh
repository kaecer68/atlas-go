#!/bin/bash
set -e

if [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
    echo "Usage: $0 [--help]"
    echo "Generate sample replay data for 2026-03-25 to 2026-03-31 at data/replay/twse_open_main.csv"
    exit 0
fi

# Generate sample replay data for 2026-03-25 to 2026-03-31
OUTPUT="data/replay/twse_open_main.csv"

echo "Date,Code,Name,TradeVolume,Open,High,Low,Close" > $OUTPUT

# Sample stocks
STOCKS=("2330 台積電" "2317 鴻海" "2454 聯發科" "2412 中華電" "2881 富邦金")

for date in 2026-03-25 2026-03-26 2026-03-27 2026-03-30 2026-03-31; do
    for stock in "${STOCKS[@]}"; do
        code=$(echo "$stock" | awk '{print $1}')
        name=$(echo "$stock" | awk '{print $2}')
        # Generate random but reasonable prices
        base_price=$((RANDOM % 500 + 100))
        volume=$((RANDOM % 100000 + 10000))
        open_price=$((base_price + RANDOM % 10 - 5))
        high_price=$((open_price + RANDOM % 5))
        low_price=$((open_price - RANDOM % 5))
        close_price=$((low_price + RANDOM % (high_price - low_price + 1)))
        
        echo "$date,$code,$name,$volume,$open_price.$RANDOM,$high_price.$RANDOM,$low_price.$RANDOM,$close_price.$RANDOM" >> $OUTPUT
    done
done

echo "Generated replay data with $(wc -l < $OUTPUT) lines"
