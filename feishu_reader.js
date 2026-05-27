const { chromium } = require('playwright');

(async () => {
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({
    storageState: JSON.stringify({ cookies: [], origins: [] }),
    viewport: { width: 1280, height: 800 }
  });
  const page = await context.newPage();
  
  try {
    await page.goto('https://acntu0o23gjz.feishu.cn/wiki/Vwaow9ZJfibLAtkyO2icv3b7n6c', { 
      waitUntil: 'domcontentloaded', 
      timeout: 60000 
    });
    
    // Wait for page content to render
    await page.waitForTimeout(5000);
    
    const title = await page.title();
    console.log('Title:', title);
    
    // Get page text content
    const text = await page.evaluate(() => document.body.innerText);
    console.log('Content:\n', text.substring(0, 5000));
    
    await page.screenshot({ path: 'feishu_doc.png', fullPage: true });
    console.log('Screenshot saved to feishu_doc.png');
  } catch (e) {
    console.error('Error:', e.message);
    // Try to get whatever is on the page
    const title = await page.title();
    console.log('Title:', title);
    const text = await page.evaluate(() => document.body.innerText);
    console.log('Content:\n', text.substring(0, 3000));
    await page.screenshot({ path: 'feishu_doc.png', fullPage: true });
  } finally {
    await browser.close();
  }
})();