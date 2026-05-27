const { chromium } = require('playwright');

(async () => {
  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage();
  
  await page.goto('https://acntu0o23gjz.feishu.cn/wiki/Vwaow9ZJfibLAtkyO2icv3b7n6c', {
    waitUntil: 'domcontentloaded',
    timeout: 60000
  });
  
  await page.waitForTimeout(8000);
  
  const title = await page.title();
  const bodyText = await page.evaluate(() => document.body.innerText);
  
  console.log('=== TITLE ===');
  console.log(title);
  console.log('\n=== CONTENT ===');
  console.log(bodyText);
  
  await browser.close();
})();