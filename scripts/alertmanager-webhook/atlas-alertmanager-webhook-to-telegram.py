#!/usr/bin/env python3
"""atlas-alertmanager-webhook-to-telegram.py

Workaround for alertmanager 0.27.0/0.28.1 + telebot.v3 404 issue.
Receives alertmanager webhook and forwards to Telegram via urllib.

Uses PLAIN TEXT (no parse_mode) so no MarkdownV2 escaping is needed —
guaranteed to work. Formatting is minimal but reliable.
"""
import json
import os
import sys
import urllib.request
import urllib.parse
import traceback
from http.server import BaseHTTPRequestHandler, HTTPServer

BOT_TOKEN = os.environ.get('TELEGRAM_BOT_TOKEN')
CHAT_ID = os.environ.get('TELEGRAM_CHAT_ID')
PORT = int(os.environ.get('PORT', '9095'))

if not BOT_TOKEN or not CHAT_ID:
    print('ERROR: TELEGRAM_BOT_TOKEN and TELEGRAM_CHAT_ID must be set', file=sys.stderr, flush=True)
    sys.exit(1)

URL = 'https://api.telegram.org/bot' + BOT_TOKEN + '/sendMessage'
NEWLINE = chr(10)


def format_message(payload):
    status = payload.get('status', 'unknown').upper()
    alerts = payload.get('alerts') or []
    lines = []
    lines.append('[{}] alertmanager webhook'.format(status))
    lines.append('Receiver: {}'.format(payload.get('receiver', '?')))
    if payload.get('commonLabels'):
        cl = payload['commonLabels']
        lines.append('Service: {}'.format(cl.get('service', '?')))
        lines.append('Severity: {}'.format(cl.get('severity', '?')))
    lines.append('')
    lines.append('Alerts ({}):'.format(len(alerts)))
    for a in alerts[:10]:
        labels = a.get('labels', {})
        ann = a.get('annotations', {})
        lines.append('- {} (sev={})'.format(labels.get('alertname', '?'), labels.get('severity', '?')))
        if ann.get('summary'):
            lines.append('  ' + ann['summary'])
        if ann.get('description'):
            desc = ann['description'][:300]
            lines.append('  ' + desc)
    if len(alerts) > 10:
        lines.append('  ... and {} more'.format(len(alerts) - 10))
    text = NEWLINE.join(lines)
    if len(text) > 4000:
        text = text[:4000] + NEWLINE + '... (truncated)'
    return text


class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        length = int(self.headers.get('Content-Length', '0'))
        raw = self.rfile.read(length) if length else b''
        try:
            payload = json.loads(raw)
        except Exception as e:
            print('JSON parse ERROR: ' + str(e), file=sys.stderr, flush=True)
            self.send_error(400, 'bad json: ' + str(e))
            return

        text = format_message(payload)
        body = urllib.parse.urlencode({'chat_id': CHAT_ID, 'text': text}).encode()
        req = urllib.request.Request(URL, data=body, headers={'Content-Type': 'application/x-www-form-urlencoded'})
        try:
            with urllib.request.urlopen(req, timeout=10) as r:
                resp_body = r.read().decode()
                if r.status == 200:
                    print('OK sent {} alerts to Telegram'.format(len(payload.get('alerts', []))), file=sys.stderr, flush=True)
                    self.send_response(200)
                    self.send_header('Content-Type', 'application/json')
                    self.end_headers()
                    self.wfile.write(b'{"ok":true}')
                else:
                    print('telegram non-200: ' + str(r.status) + ' ' + resp_body[:300], file=sys.stderr, flush=True)
                    self.send_error(500, resp_body)
        except urllib.error.HTTPError as e:
            err_body = e.read().decode()[:300]
            print('telegram HTTPError ' + str(e.code) + ': ' + err_body, file=sys.stderr, flush=True)
            self.send_error(502, str(e))
        except Exception as e:
            print('telegram ERROR: ' + str(e) + NEWLINE + traceback.format_exc(), file=sys.stderr, flush=True)
            self.send_error(502, str(e))

    def log_message(self, format, *args):
        pass


if __name__ == '__main__':
    print('atlas-webhook-to-telegram listening on :' + str(PORT) + ' (chat_id=' + CHAT_ID + ')', file=sys.stderr, flush=True)
    HTTPServer(('0.0.0.0', PORT), Handler).serve_forever()
