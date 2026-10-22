// 日期格式化：固定按 Asia/Shanghai 显示，不依赖浏览器本地时区。
(function (root) {
  const DISPLAY_TZ = 'Asia/Shanghai';
  function formatDate(s) {
    if (!s) return '—';
    const d = new Date(s);
    if (isNaN(d.getTime())) return '—';
    return d.toLocaleString('zh-CN', {
      timeZone: DISPLAY_TZ, month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false,
    });
  }
  root.formatDate = formatDate;
  root.DISPLAY_TZ = DISPLAY_TZ;
  if (typeof module !== 'undefined' && module.exports) module.exports = { formatDate, DISPLAY_TZ };
})(typeof window !== 'undefined' ? window : globalThis);
