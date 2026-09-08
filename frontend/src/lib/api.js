import { getLocale, translateServerMessage } from './i18n/index.js';
import { storageError } from './stores.js';

export async function api(method, url, body, headers = {}) {
  const locale = getLocale();
  const opts = {
    method,
    headers: {
      'Content-Type': 'application/json',
      ...headers,
      'X-UI-Locale': locale,
      'Accept-Language': locale === 'en' ? 'en-US,en;q=0.9' : 'zh-CN,zh;q=0.9',
    },
  };
  if (body) opts.body = JSON.stringify(body);
  const r = await fetch(url, opts);
  const data = await r.json();
  if (!r.ok) {
    const raw = data.error || 'Request failed';
    if (data.code === 'storage_save_failed' && data.storage_error) {
      storageError.set(data.storage_error);
    }
    // Backend mostly responds in Chinese today; translate known strings on the client.
    throw new Error(translateServerMessage(raw, locale));
  }
  return data;
}
