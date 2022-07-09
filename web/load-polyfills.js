// Polyfill needed to use adoptedStyleSheets and replaceSync() on Firefox and Safari:
// https://caniuse.com/mdn-api_cssstylesheet_replacesync
// https://caniuse.com/?search=adoptedStyleSheets
(async () => {
  if (!('adoptedStyleSheets' in document)) {
    await import('./construct-style-sheets-polyfill.js');
  }
})();
