// 运行：TZ=Asia/Tokyo node --test frontend/tests/  （任意时区都应输出北京时间）
const test = require('node:test');
const assert = require('node:assert');
const { formatDate } = require('../js/datefmt.js');

test('formatDate 固定按 Asia/Shanghai 显示', () => {
  assert.strictEqual(formatDate('2026-10-29T17:28:05+08:00'), '10/29 17:28');
  assert.strictEqual(formatDate('2026-10-29T09:28:05Z'), '10/29 17:28');
  assert.strictEqual(formatDate('2026-12-31T16:30:00Z'), '01/01 00:30');
  assert.strictEqual(formatDate(null), '—');
  assert.strictEqual(formatDate('bad'), '—');
});
