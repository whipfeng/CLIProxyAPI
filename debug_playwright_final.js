const { chromium } = require('playwright');

(async () => {
  const browser = await chromium.launch({ headless: false, slowMo: 300 });
  const page = await browser.newPage();
  
  console.log('=== 开始调试 Context Length Badge 问题 (最终版) ===\n');
  
  try {
    // 步骤 1: 导航到管理页面
    console.log('步骤 1: 导航到管理页面...');
    await page.goto('http://localhost:8317/management.html', { waitUntil: 'networkidle', timeout: 30000 });
    await page.waitForTimeout(2000);
    await page.screenshot({ path: 'final_step1_initial.png' });
    
    // 步骤 2: 登录（使用精确的选择器）
    console.log('\n步骤 2: 执行登录...');
    
    // 精确查找密码输入框（通过 id 或 placeholder）
    const passwordInput = await page.$('#_r_1_, input[placeholder*="管理密钥"], input[placeholder*="密钥"], input[type="password"]');
    
    if (passwordInput) {
      await passwordInput.click();
      await passwordInput.fill('');
      await page.waitForTimeout(300);
      
      // 使用 type 模拟真实输入
      await passwordInput.type('sk-admin-key', { delay: 50 });
      console.log('已输入管理密钥');
      
      await page.waitForTimeout(500);
      await page.screenshot({ path: 'final_step2_input.png' });
      
      // 精确查找登录提交按钮（type=submit 且文本为"登录"）
      // 重要：不要点击其他按钮！
      const loginButton = await page.evaluateHandle(() => {
        const buttons = Array.from(document.querySelectorAll('button'));
        // 查找 type=submit 且包含"登录"文本的按钮
        return buttons.find(btn => 
          btn.type === 'submit' && 
          btn.textContent?.trim() === '登录'
        );
      });
      
      if (loginButton) {
        console.log('找到登录按钮 (type=submit, text="登录")');
        await loginButton.click();
        console.log('已点击登录按钮');
        
        // 等待页面响应
        console.log('等待登录响应...');
        
        // 等待 URL 变化或页面内容变化
        try {
          await Promise.race([
            page.waitForURL('**/management.html#/**', { timeout: 8000 }),
            page.waitForSelector('input[type="password"]', { state: 'hidden', timeout: 8000 })
          ]).catch(() => {});
        } catch (e) {}
        
        await page.waitForTimeout(2000);
        await page.screenshot({ path: 'final_step3_after_login.png', fullPage: true });
        
        // 验证登录状态
        const currentUrl = page.url();
        const isLoggedIn = !currentUrl.includes('/login');
        console.log(`当前URL: ${currentUrl}`);
        console.log(`登录状态: ${isLoggedIn ? '成功 ✓' : '仍在登录页面 ⚠️'}`);
        
        if (!isLoggedIn) {
          // 检查是否有错误提示
          const errorText = await page.evaluate(() => {
            const errorEls = document.querySelectorAll('[class*="error"], [class*="alert"], [class*="toast"]');
            return Array.from(errorEls).map(el => el.textContent?.trim()).filter(t => t).join('; ');
          });
          if (errorText) {
            console.log(`错误信息: ${errorText}`);
          }
        }
        
      } else {
        console.log('未找到登录按钮');
        // 列出所有按钮供调试
        const allButtons = await page.evaluate(() => {
          return Array.from(document.querySelectorAll('button')).map((btn, i) => ({
            index: i,
            type: btn.type,
            text: btn.textContent?.trim(),
            classList: Array.from(btn.classList)
          }));
        });
        console.log('所有按钮:', JSON.stringify(allButtons, null, 2));
      }
    }
    
    // 步骤 3: 如果已登录，导航到 AI Providers
    if (!page.url().includes('/login')) {
      console.log('\n步骤 3: 导航到 AI Providers...');
      
      // 查找侧边栏或导航菜单
      const navItems = await page.evaluate(() => {
        const items = document.querySelectorAll('nav a, [role="navigation"] a, sidebar a, [class*="sidebar"] a, [class*="nav"] a, [class*="menu"] a');
        return Array.from(items)
          .filter(a => a.offsetParent !== null)
          .map(a => ({
            text: a.textContent?.trim(),
            href: a.href || a.getAttribute('href'),
            className: a.className?.toString()
          }))
          .slice(0, 20);
      });
      
      console.log('导航项:', JSON.stringify(navItems, null, 2));
      
      // 查找包含 "Provider" 的导航项
      const providerNav = navItems.find(item => 
        item.text.toLowerCase().includes('provider') || 
        item.text.includes('提供商')
      );
      
      if (providerNav) {
        console.log(`点击: "${providerNav.text}"`);
        await page.click(`a:text-is("${providerNav.text}")`);
        await page.waitForTimeout(2000);
        await page.screenshot({ path: 'final_step4_providers.png', fullPage: true });
      }
      
      // 步骤 4: 查找并进入 Trae provider
      console.log('\n步骤 4: 查找 Trae Provider...');
      
      // 在页面上搜索 Trae 相关内容
      const traeElements = await page.evaluate(() => {
        // 搜索所有可见元素中包含 "Trae" 文本的元素
        const walker = document.createTreeWalker(
          document.body,
          NodeFilter.SHOW_TEXT,
          null,
          false
        );
        
        const results = [];
        let node;
        while (node = walker.nextNode()) {
          if (node.textContent.toLowerCase().includes('trae') && 
              node.parentElement.offsetParent !== null &&
              node.textContent.trim().length < 100) {
            results.push({
              text: node.textContent.trim(),
              tag: node.parentElement.tagName,
              parentClass: node.parentElement.className?.toString(),
              grandParentClass: node.parentElement.parentElement?.className?.toString()
            });
          }
        }
        return results.slice(0, 15);
      });
      
      console.log(`找到 ${traeElements.length} 个 Trae 相关文本节点:`);
      traeElements.forEach((el, i) => {
        console.log(`${i + 1}. "${el.text}" in <${el.tag}> class="${el.parentClass}"`);
      });
      
      if (traeElements.length > 0) {
        // 尝试找到可点击的 Trae 行/卡片
        console.log('\n尝试进入 Trae 编辑页面...');
        
        // 方法1: 直接点击包含 "Trae" 的行或卡片
        const traeClicked = await page.evaluate(() => {
          // 查找包含 "Trae" 文本的可点击容器
          const allElements = document.querySelectorAll('*');
          
          for (const el of allElements) {
            if (el.textContent?.toLowerCase().includes('trae') && 
                el.children.length > 0 && 
                el.children.length < 20 &&
                el.offsetParent !== null) {
              
              // 检查是否是容器元素（不是纯文本）
              const isContainer = ['DIV', 'LI', 'TR', 'ARTICLE', 'SECTION'].includes(el.tagName);
              
              if (isContainer) {
                // 尝试在这个容器内查找编辑/详情按钮
                const editBtn = el.querySelector('[class*="edit"] button, button[class*="edit"], a[class*="edit"]');
                if (editBtn) {
                  editBtn.click();
                  return { clicked: true, method: 'edit-button' };
                }
                
                // 或者直接点击容器本身
                el.click();
                return { clicked: true, method: 'container-click' };
              }
            }
          }
          return { clicked: false };
        });
        
        if (traeClicked.clicked) {
          console.log(`✓ 已点击 (方法: ${traeClicked.method})`);
          await page.waitForTimeout(2000);
          await page.screenshot({ path: 'final_step5_trae_edit.png', fullPage: true });
        } else {
          console.log('未能自动点击 Trae 元素');
        }
      }
      
      // 步骤 5: 查找导入模型按钮
      console.log('\n步骤 5: 查找导入模型功能...');
      
      // 收集 API 响应数据
      const capturedResponses = [];
      
      page.on('response', async (response) => {
        const url = response.url();
        // 监听 import 相关的 API 调用
        if (url.includes('/import') || url.includes('/api/')) {
          try {
            const contentType = response.headers()['content-type'] || '';
            if (contentType.includes('json')) {
              const jsonData = await response.json();
              capturedResponses.push({
                url: url,
                status: response.status(),
                data: jsonData,
                timestamp: new Date().toISOString()
              });
              
              console.log(`\n[API 响应] ${url}`);
              console.log(`  状态码: ${response.status()}`);
              
              // 特别检查 context_length
              const respStr = JSON.stringify(jsonData);
              if (respStr.includes('context_length')) {
                console.log('  ✓ 发现 context_length 字段！');
                
                // 提取模型列表中的 context_length
                const extractContextLengths = (obj, path = '') => {
                  if (Array.isArray(obj)) {
                    obj.forEach((item, idx) => extractContextLengths(item, `${path}[${idx}]`));
                  } else if (typeof obj === 'object' && obj !== null) {
                    if ('context_length' in obj) {
                      console.log(`    ${path}: context_length = ${obj.context_length} (模型: ${obj.name || obj.id || 'unknown'})`);
                    }
                    Object.keys(obj).forEach(key => extractContextLengths(obj[key], `${path}.${key}`));
                  }
                };
                
                extractContextLengths(jsonData);
              }
            }
          } catch (e) {
            // 忽略解析错误
          }
        }
      });
      
      // 查找导入按钮
      const importBtnInfo = await page.evaluate(() => {
        const buttons = document.querySelectorAll('button, a, [role="button"]');
        return Array.from(buttons)
          .filter(el => el.offsetParent !== null)
          .map(el => ({
            tag: el.tagName,
            text: el.textContent?.trim(),
            className: el.className?.toString(),
            hasImportKeyword: (
              el.textContent?.toLowerCase().includes('import') ||
              el.textContent?.includes('导入') ||
              el.className?.toString().toLowerCase().includes('import')
            )
          }))
          .filter(el => el.hasImportKeyword || el.text.length > 0);
      });
      
      console.log(`\n找到 ${importBtnInfo.filter(b => b.hasImportKeyword).length} 个导入相关按钮:`);
      importBtnInfo.filter(b => b.hasImportKeyword).forEach((btn, i) => {
        console.log(`${i + 1}. [${btn.tag}] "${btn.text}" | class: ${btn.className}`);
      });
      
      // 点击第一个导入按钮
      const importBtn = importBtnInfo.find(b => b.hasImportKeyword);
      if (importBtn) {
        console.log(`\n点击导入按钮: "${importBtn.text}"`);
        
        try {
          await page.click(`button:has-text("${importBtn.text}"), a:has-text("${importBtn.text}")`, { timeout: 5000 });
          console.log('已点击导入按钮');
          
          // 等待导入完成
          console.log('等待导入完成...');
          await page.waitForTimeout(15000); // 等待足够长的时间让导入完成
          
          await page.screenshot({ path: 'final_step6_after_import.png', fullPage: true });
          console.log('截图: final_step6_after_import.png');
          
        } catch (e) {
          console.log('点击导入按钮失败:', e.message);
        }
      } else {
        console.log('未找到导入按钮');
        console.log('\n页面上所有按钮:');
        importBtnInfo.slice(0, 20).forEach((btn, i) => {
          console.log(`${i + 1}. [${btn.tag}] "${btn.text}"`);
        });
      }
      
      // 步骤 6: 最终检查
      console.log('\n' + '='.repeat(70));
      console.log('最终检查结果');
      console.log('='.repeat(70));
      
      // 6a. badge 元素数量
      const badgeCount = await page.evaluate(() => {
        return document.querySelectorAll('.model-context-length-badge').length;
      });
      console.log(`\n[检查 6a] .model-context-length-badge 元素数量: ${badgeCount}`);
      
      // 6b. 所有包含 badge/context 的元素
      const badgeElements = await page.evaluate(() => {
        const allElements = document.querySelectorAll('*');
        return Array.from(allElements).filter(el => {
          const cls = (el.className?.toString() || '').toLowerCase;
          const hasBadgeOrContext = cls.includes('badge') || cls.includes('context-length') || cls.includes('context_length');
          return hasBadgeOrContext && el.children.length === 0 && el.textContent?.trim();
        }).map(el => ({
          tag: el.tagName,
          class: el.className,
          text: el.textContent?.trim(),
          outerHTML: el.outerHTML.substring(0, 250)
        }));
      });
      console.log(`\n[检查 6b] 包含 badge/context-length 类名的元素数量: ${badgeElements.length}`);
      badgeElements.forEach((el, i) => {
        console.log(`\n  元素 ${i + 1}:`);
        console.log(`    标签: <${el.tag}>`);
        console.log(`    类名: ${el.class}`);
        console.log(`    文本: "${el.text}"`);
        console.log(`    HTML: ${el.outerHTML}`);
      });
      
      // 6c. 页面模型区域结构
      const modelAreaStructure = await page.evaluate(() => {
        // 查找可能的模型展示区域
        const areas = [];
        
        // 表格
        document.querySelectorAll('table').forEach(table => {
          if (table.offsetParent !== null) {
            areas.push({
              type: 'table',
              rows: table.querySelectorAll('tr').length,
              htmlPreview: table.outerHTML.substring(0, 500),
              textSample: table.textContent?.substring(0, 200)
            });
          }
        });
        
        // 列表容器
        document.querySelectorAll('[class*="list"], [class*="grid"]').forEach(list => {
          if (list.offsetParent !== null && list.children.length > 3) {
            areas.push({
              type: 'list/container',
              childCount: list.children.length,
              className: list.className?.toString(),
              textSample: list.textContent?.substring(0, 200)
            });
          }
        });
        
        return areas.slice(0, 5);
      });
      console.log(`\n[检查 6c] 模型展示区域结构 (${modelAreaStructure.length}个):`);
      modelAreaStructure.forEach((area, i) => {
        console.log(`\n  区域 ${i + 1} (${area.type}):`);
        if (area.rows) console.log(`    行数: ${area.rows}`);
        if (area.childCount) console.log(`    子元素数: ${area.childCount}`);
        if (area.className) console.log(`    类名: ${area.className}`);
        console.log(`    文本预览: ${area.textSample?.substring(0, 150)}...`);
      });
      
      // 最终截图
      await page.screenshot({ path: 'final_state_complete.png', fullPage: true });
      console.log('\n最终完整截图: final_state_complete.png');
      
      // 总结
      console.log('\n' + '='.repeat(70));
      console.log('调试总结报告');
      console.log('='.repeat(70));
      console.log(`
1. Badge 元素 (.model-context-length-badge): ${badgeCount} 个
   ${badgeCount > 0 ? '✓ 存在 badge 元素' : '✗ 未找到 badge 元素'}
   
2. 包含 badge/context 类名的元素: ${badgeElements.length} 个
   ${badgeElements.length > 0 ? '✓ 找到相关元素' : '✗ 未找到任何相关元素'}
   
3. API 响应记录:
   - 共捕获 ${capturedResponses.length} 条 API 响应
   - 包含 context_length 的响应: ${capturedResponses.filter(r => JSON.stringify(r.data).includes('context_length')).length} 条
   
4. 生成的截图文件:
   - final_step1_initial.png (初始页面)
   - final_step2_input.png (输入密钥后)
   - final_step3_after_login.png (登录后)
   - final_step4_providers.png (Providers 页面)
   - final_step5_trae_edit.png (Trae 编辑页面)
   - final_step6_after_import.png (导入后)
   - final_state_complete.png (最终状态)

5. 问题诊断:
   ${badgeCount === 0 ? 
     '⚠️ Context Length Badge 功能未正常工作 - 未在 DOM 中找到预期的 badge 元素' :
     '✓ Context Length Badge 功能似乎正常工作'}
      `);
      console.log('='.repeat(70));
      
    } else {
      console.log('\n⚠️ 未能成功登录，无法继续后续调试步骤');
      console.log('请检查管理密钥是否正确');
    }
    
    // 保持浏览器打开以便查看
    console.log('\n浏览器将在 10 秒后关闭...');
    await page.waitForTimeout(10000);
    
  } catch (error) {
    console.error('\n发生严重错误:', error.message);
    console.error(error.stack);
    await page.screenshot({ path: 'final_error.png', fullPage: true });
    console.log('错误截图: final_error.png');
  } finally {
    await browser.close();
    console.log('\n调试会话结束！');
  }
})();
