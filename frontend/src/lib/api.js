import { getLocale, translate, translateServerMessage } from './i18n/index.js';
import { storageError } from './stores.js';

export async function apiFetch(url, opts = {}) {
  const locale = getLocale();
  const r = await fetch(url, {
    ...opts,
    headers: {
      ...opts.headers,
      'X-UI-Locale': locale,
      'Accept-Language': locale === 'en' ? 'en-US,en;q=0.9' : 'zh-CN,zh;q=0.9',
    },
  });
  let data;
  if (!r.ok) {
    try { data = await r.json(); } catch (_) { data = {}; }
    const raw = data.error || translate('common.requestFailed', null, locale);
    if (data.code === 'storage_save_failed' && data.storage_error) {
      storageError.set(data.storage_error);
    }
    // Backend mostly responds in Chinese today; translate known strings on the client.
    throw new Error(translateServerMessage(raw, locale));
  }
  return r;
}

export async function api(method, url, body, headers = {}) {
  const opts = { method, headers: { 'Content-Type': 'application/json', ...headers } };
  if (body) opts.body = JSON.stringify(body);
  return (await apiFetch(url, opts)).json();
}
