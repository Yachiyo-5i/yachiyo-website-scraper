import http from 'node:http';
import { chromium } from 'playwright';

const host = process.env.HOST || '0.0.0.0';
const port = Number(process.env.PORT || '3001');
const defaultTimeout = Number(process.env.PLAYWRIGHT_TIMEOUT || '60000');
const browserEndpoint = process.env.PLAYWRIGHT_WS_URL || '';
const userAgent = process.env.USER_AGENT ||
  'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36';

let browserPromise;

async function getBrowser() {
  if (!browserPromise) {
    browserPromise = browserEndpoint
      ? chromium.connect(normalizeBrowserEndpoint(browserEndpoint))
      : chromium.launch({ headless: true });
  }
  return browserPromise;
}

function normalizeBrowserEndpoint(value) {
  if (value.startsWith('http://')) return value.replace(/^http:\/\//, 'ws://').replace(/\/+$/, '/');
  if (value.startsWith('https://')) return value.replace(/^https:\/\//, 'wss://').replace(/\/+$/, '/');
  return value;
}

function readJSON(req) {
  return new Promise((resolve, reject) => {
    let body = '';
    req.setEncoding('utf8');
    req.on('data', chunk => {
      body += chunk;
      if (body.length > 2 * 1024 * 1024) {
        reject(new Error('request body too large'));
        req.destroy();
      }
    });
    req.on('end', () => {
      if (!body.trim()) {
        resolve({});
        return;
      }
      try {
        resolve(JSON.parse(body));
      } catch (err) {
        reject(err);
      }
    });
    req.on('error', reject);
  });
}

function writeJSON(res, status, payload) {
  const encoded = JSON.stringify(payload);
  res.writeHead(status, {
    'content-type': 'application/json; charset=utf-8',
    'content-length': Buffer.byteLength(encoded)
  });
  res.end(encoded);
}

async function applyCookieHeader(context, targetURL, cookieHeader) {
  if (!cookieHeader || typeof cookieHeader !== 'string') return;
  const url = new URL(targetURL);
  const cookies = cookieHeader.split(';').map(part => {
    const [rawName, ...rest] = part.split('=');
    const name = rawName?.trim();
    if (!name) return null;
    return {
      name,
      value: rest.join('=').trim(),
      domain: url.hostname,
      path: '/'
    };
  }).filter(Boolean);
  if (cookies.length > 0) {
    await context.addCookies(cookies);
  }
}

async function settleSehuatangAgeGate(context, page, targetURL) {
  const target = new URL(targetURL);
  if (!/(^|\.)sehuatang\.org$/i.test(target.hostname)) return null;

  const safeID = await page.evaluate(() => globalThis.safeid || '').catch(() => '');
  if (!safeID) return null;

  await context.addCookies([{
    name: '_safe',
    value: String(safeID),
    domain: '.sehuatang.org',
    path: '/'
  }]);
  return page.goto(targetURL, { waitUntil: 'domcontentloaded', timeout: defaultTimeout });
}

function normalizeAutoclick(value) {
  if (!value) return null;
  const xpath = typeof value === 'string' ? value : value.xpath;
  if (!xpath || typeof xpath !== 'string' || !xpath.trim()) return null;
  return {
    xpath: xpath.trim(),
    timeout: Number(value.timeout || value.timeout_ms || 1000),
    settleMs: Number(value.settle_ms || 500)
  };
}

async function maybeAutoclick(page, autoclick, timeout) {
  const cfg = normalizeAutoclick(autoclick);
  if (!cfg) return null;

  const clickTimeout = Math.max(1, Math.min(timeout, cfg.timeout));
  const element = page.locator(`xpath=${cfg.xpath}`).first();
  const visible = await element.waitFor({ state: 'visible', timeout: clickTimeout })
    .then(() => true)
    .catch(() => false);
  if (!visible) return null;

  const navigation = page.waitForNavigation({
    waitUntil: 'domcontentloaded',
    timeout: clickTimeout
  }).catch(() => null);
  await element.click({ timeout: clickTimeout });
  const response = await navigation;
  await page.waitForLoadState('networkidle', { timeout: Math.min(timeout, 15000) }).catch(() => {});
  await page.waitForTimeout(Math.max(0, cfg.settleMs));
  return response;
}

async function fetchPage(payload) {
  if (!payload.url) {
    throw new Error('url is required');
  }

  const timeout = Number(payload.timeout || defaultTimeout);
  const browser = await getBrowser();
  const context = await browser.newContext({
    userAgent,
    locale: 'zh-CN',
    extraHTTPHeaders: payload.headers || {}
  });
  await applyCookieHeader(context, payload.url, payload.cookies);

  const page = await context.newPage();
  try {
    const method = String(payload.method || 'GET').toUpperCase();
    if (method !== 'GET') {
      await page.setExtraHTTPHeaders({ ...(payload.headers || {}) });
    }

    let response = await page.goto(payload.url, { waitUntil: 'domcontentloaded', timeout });
    response = await maybeAutoclick(page, payload.autoclick, timeout) || response;
    response = await settleSehuatangAgeGate(context, page, payload.url) || response;
    await page.waitForLoadState('networkidle', { timeout: Math.min(timeout, 15000) }).catch(() => {});
    await page.waitForTimeout(Number(payload.settle_ms || 500));

    const html = await page.content();
    const headers = {};
    for (const [key, value] of Object.entries(response?.headers() || {})) {
      headers[key] = value;
    }
    return {
      status: response?.status() || 200,
      final_url: page.url(),
      headers,
      body: html
    };
  } finally {
    await context.close();
  }
}

const server = http.createServer(async (req, res) => {
  if (req.method === 'GET' && req.url === '/health') {
    writeJSON(res, 200, { ok: true });
    return;
  }
  if (req.method !== 'POST' || req.url !== '/fetch') {
    writeJSON(res, 404, { error: 'not found' });
    return;
  }

  try {
    const payload = await readJSON(req);
    const result = await fetchPage(payload);
    writeJSON(res, 200, result);
  } catch (err) {
    writeJSON(res, 200, { error: err?.message || String(err) });
  }
});

server.listen(port, host, () => {
  console.log(`playwright fetcher listening on ${host}:${port}`);
});

process.on('SIGTERM', async () => {
  server.close();
  const browser = await browserPromise.catch(() => null);
  await browser?.close().catch(() => {});
  process.exit(0);
});
