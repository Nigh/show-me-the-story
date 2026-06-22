/**
 * Prose unit count — mirrors backend prose_units.go.
 * CJK +1 each; Latin alnum tokens +1 (with internal . , - # glue); punct/space break tokens.
 */

function normalizeWideAlnum(code) {
  if (code >= 0xff10 && code <= 0xff19) return code - 0xff10 + 0x30; // ０-９
  if (code >= 0xff21 && code <= 0xff3a) return code - 0xff21 + 0x41; // Ａ-Ｚ
  if (code >= 0xff41 && code <= 0xff5a) return code - 0xff41 + 0x61; // ａ-ｚ
  return code;
}

function isLatinAlnum(code) {
  return (code >= 0x41 && code <= 0x5a) || (code >= 0x61 && code <= 0x7a) || (code >= 0x30 && code <= 0x39);
}

function isTokenGlue(code) {
  return code === 0x2e || code === 0x2c || code === 0x2d || code === 0x23; // . , - #
}

function isWhitespace(code) {
  return /\s/u.test(String.fromCodePoint(code));
}

const CJK_PUNCT = new Set([
  0x3002, 0xff0c, 0x3001, 0xff1b, 0xff1a, 0xff01, 0xff1f, 0x2026, 0x2014,
  0x300c, 0x300d, 0x300e, 0x300f, 0xff08, 0xff09, 0x300a, 0x300b, 0x3010, 0x3011,
  0x201c, 0x201d, 0x2018, 0x2019, 0xb7,
]);

function isCJKPunct(code) {
  return CJK_PUNCT.has(code);
}

function isLatinBreakPunct(code) {
  return (
    code === 0x2e || code === 0x2c || code === 0x3b || code === 0x3a || code === 0x21 || code === 0x3f ||
    code === 0x28 || code === 0x29 || code === 0x5b || code === 0x5d || code === 0x7b || code === 0x7d ||
    code === 0x22 || code === 0x27 || code === 0x2f || code === 0x5c || code === 0x40 || code === 0x24 ||
    code === 0x25 || code === 0x5e || code === 0x26 || code === 0x2a || code === 0x2b || code === 0x3d ||
    code === 0x7c || code === 0x7e || code === 0x60 || code === 0x3c || code === 0x3e
  );
}

function isBreakPunct(code) {
  return isCJKPunct(code) || isLatinBreakPunct(code);
}

function isCJK(code) {
  return (
    (code >= 0x4e00 && code <= 0x9fff) ||
    (code >= 0x3400 && code <= 0x4dbf) ||
    (code >= 0x3040 && code <= 0x309f) ||
    (code >= 0x30a0 && code <= 0x30ff) ||
    (code >= 0xac00 && code <= 0xd7af)
  );
}

export function countProseUnits(text) {
  if (!text) return 0;
  let count = 0;
  let inToken = false;
  for (const ch of text) {
    let code = normalizeWideAlnum(ch.codePointAt(0));
    if (isCJK(code)) {
      if (inToken) {
        count++;
        inToken = false;
      }
      count++;
    } else if (isLatinAlnum(code)) {
      inToken = true;
    } else if (isTokenGlue(code)) {
      if (inToken) continue;
    } else if (isWhitespace(code) || isBreakPunct(code)) {
      if (inToken) {
        count++;
        inToken = false;
      }
    } else {
      if (inToken) {
        count++;
        inToken = false;
      }
    }
  }
  if (inToken) count++;
  return count;
}
