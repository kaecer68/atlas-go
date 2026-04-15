#!/usr/bin/env python3
"""
Merge TWSE daily CSV into the main replay dataset.
Usage:
  python scripts/merge-twse-csv.py --source /path/to/twse_20260401.csv --target data/replay/tw_extended_90days.csv
"""
import argparse
import csv
import os

STOCK_NAMES = {
    '0050': '元大台灣50', '0056': '元大高股息', '1301': '台塑', '1303': '南亞', '1326': '台化',
    '2303': '聯電', '2308': '台達電', '2317': '鴻海', '2330': '台積電', '2382': '廣達',
    '2454': '聯發科', '2603': '長榮', '2609': '陽明', '2615': '萬海', '2881': '富邦金',
    '2882': '國泰金', '2886': '兆豐金', '2891': '中信金', '2892': '第一金', '3008': '大立光',
    '3034': '聯詠', '3037': '欣興', '6669': '緯穎',
}

def main():
    parser = argparse.ArgumentParser(description='Merge TWSE CSV into replay dataset')
    parser.add_argument('--source', required=True, help='Source TWSE CSV file')
    parser.add_argument('--target', default='data/replay/tw_extended_90days.csv', help='Target replay CSV')
    args = parser.parse_args()

    target_codes = set(STOCK_NAMES.keys())
    existing = set()
    if os.path.exists(args.target):
        with open(args.target, 'r', encoding='utf-8') as f:
            reader = csv.DictReader(f)
            for row in reader:
                existing.add((row['Date'], row['Code']))

    new_rows = []
    with open(args.source, 'r', encoding='utf-8') as f:
        reader = csv.DictReader(f)
        for row in reader:
            code = row.get('證券代號', row.get('Code', '')).strip()
            if code not in target_codes:
                continue
            date = row.get('日期', row.get('Date', '')).strip()
            date = date.replace('/', '-')
            if len(date) == 8 and date.isdigit():
                date = f"{date[:4]}-{date[4:6]}-{date[6:]}"
            key = (date, code)
            if key in existing:
                continue
            new_rows.append({
                'Date': date,
                'Code': code,
                'Name': STOCK_NAMES.get(code, ''),
                'TradeVolume': int(row.get('成交股數', row.get('TradeVolume', '0')).replace(',', '')),
                'Open': float(row.get('開盤價', row.get('Open', '0')).replace(',', '')),
                'High': float(row.get('最高價', row.get('High', '0')).replace(',', '')),
                'Low': float(row.get('最低價', row.get('Low', '0')).replace(',', '')),
                'Close': float(row.get('收盤價', row.get('Close', '0')).replace(',', '')),
            })
            existing.add(key)

    if not new_rows:
        print('No new rows to merge.')
        return

    file_exists = os.path.exists(args.target) and os.path.getsize(args.target) > 0
    with open(args.target, 'a', encoding='utf-8', newline='') as f:
        writer = csv.DictWriter(f, fieldnames=['Date', 'Code', 'Name', 'TradeVolume', 'Open', 'High', 'Low', 'Close'])
        if not file_exists:
            writer.writeheader()
        writer.writerows(new_rows)
    print(f'Merged {len(new_rows)} rows from {args.source} into {args.target}')

if __name__ == '__main__':
    main()
