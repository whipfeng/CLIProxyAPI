const { chromium } = require('playwright');

(async () => {
  const browser = await chromium.launch({ headless: false });
  const page = await browser.newPage();
  
  console.log('=== 开始调试 Context Length Badge 问题 ===\n');
  
  try {
    // 步骤 1: 导航到管理页面
    console.log('步骤 1: 导航到 http://localhost:8317/management.html');
    await page.goto('http://localhost:8317/management.html', { waitUntil: 'networkidle' });
    await page.waitForTimeout(2000);
    
    // 截图 - 初始状态
    await page.screenshot({ path: 'debug_step1_initial.png', fullPage: true });
    console.log('截图已保存: debug_step1_initial.png');
    
    // 检查是否有登录页面
    const pageTitle = await page.title();
    console.log(`页面标题: ${pageTitle}`);
    
    const pageContent = await page.content();
    
    // 检查是否需要登录
    if (pageContent.includes('login') || pageContent.includes('password') || pageContent.includes('管理密钥')) {
      console.log('\n检测到登录页面，尝试查找输入框...');
      
      // 尝试查找密码/管理密钥输入框
      const passwordInput = await page.$('input[type="password"], input[name*="key"], input[name*="password"], input[placeholder*="密钥"], input[placeholder*="密码"]');
      
      if (passwordInput) {
        console.log('找到密码输入框，尝试输入管理密钥...');
        // 使用正确的管理密钥
        await passwordInput.fill('sk-admin-key');
        
        // 点击提交按钮
        const submitButton = await page.$('button[type="submit"], button:has-text("登录"), button:has-text("Login")');
        if (submitButton) {
          await submitButton.click();
          console.log('已点击登录按钮');
          await page.waitForTimeout(3000);
        }
      } else {
        console.log('未找到密码输入框，可能需要其他方式登录');
      }
    } else {
      console.log('未检测到明显的登录表单');
    }
    
    // 截图 - 登录后状态
    await page.screenshot({ path: 'debug_step2_after_login.png', fullPage: true });
    console.log('\n截图已保存: debug_step2_after_login.png');
    
    // 步骤 3: 查找并点击 AI Providers 部分
    console.log('\n步骤 3: 查找 AI Providers 部分...');
    
    // 查找包含 "AI Providers" 或 "提供商" 或 "Providers" 的元素
    const providersLink = await page.$('a:has-text("AI Provider"), a:has-text("提供商"), a:has-text("Provider"), [class*="provider"]:visible, button:has-text("AI Provider")');
    
    if (providersLink) {
      console.log('找到 AI Providers 链接，点击中...');
      await providersLink.click();
      await page.waitForTimeout(2000);
    } else {
      console.log('未直接找到 AI Providers 链接，尝试其他选择器...');
      // 打印页面上所有可见的链接和按钮
      const links = await page.evaluate(() => {
        return Array.from(document.querySelectorAll('a, button, [role="tab"], [role="menuitem"]'))
          .filter(el => el.offsetParent !== null) // 可见
          .map(el => ({ tag: el.tagName, text: el.textContent?.trim(), class: el.className }))
          .slice(0, 30);
      });
      console.log('页面上的可交互元素:', JSON.stringify(links, null, 2));
    }
    
    // 截图 - AI Providers 页面
    await page.screenshot({ path: 'debug_step3_providers.png', fullPage: true });
    console.log('\n截图已保存: debug_step3_providers.png');
    
    // 步骤 4: 查找 Trae provider 并进入编辑页面
    console.log('\n步骤 4: 查找 Trae provider...');
    
    // 查找包含 "Trae" 或 "trae" 的元素
    const traeElement = await page.$('[class*="trae"]:visible, [data-provider*="trae"]:visible, *:has-text("Trae"):not(script):not(style)');
    
    if (traeElement) {
      console.log('找到 Trae 相关元素');
      // 可能需要点击编辑按钮或进入详情
      const editButton = await page.$('[class*="trae"] button:has-text("编辑"), [class*="trae"] button:has-text("Edit"), [class*="trae"] a:has-text("编辑"), [class*="trae"] a:has-text("Edit"), [class*="trae"] [class*="edit"]');
      
      if (editButton) {
        console.log('找到编辑按钮，点击中...');
        await editButton.click();
        await page.waitForTimeout(2000);
      } else {
        // 直接点击 Trae 元素本身
        console.log('未找到编辑按钮，尝试点击 Trae 元素...');
        await traeElement.click();
        await page.waitForTimeout(2000);
      }
    } else {
      console.log('未找到 Trae 元素，列出所有 provider...');
      const allProviders = await page.evaluate(() => {
        return Array.from(document.querySelectorAll('*'))
          .filter(el => el.textContent?.includes('Trae') || el.textContent?.includes('trae') || 
                       el.className?.toString().toLowerCase().includes('provider'))
          .slice(0, 20)
          .map(el => ({
            tag: el.tagName,
            text: el.textContent?.trim().substring(0, 100),
            class: el.className
          }));
      });
      console.log('可能的 provider 元素:', JSON.stringify(allProviders, null, 2));
    }
    
    // 截图 - Trae 编辑页面
    await page.screenshot({ path: 'debug_step4_trae_edit.png', fullPage: true });
    console.log('\n截图已保存: debug_step4_trae_edit.png');
    
    // 步骤 5: 点击 "Import Models" / "导入模型" 按钮
    console.log('\n步骤 5: 查找并点击导入模型按钮...');
    
    const importButton = await page.$('button:has-text("导入模型"), button:has-text("Import Models"), button:has-text("Import"), [class*="import"]');
    
    if (importButton) {
      console.log('找到导入模型按钮，点击中...');
      
      // 监听网络请求
      let importResponse = null;
      page.on('response', async (response) => {
        if (response.url().includes('/import')) {
          importResponse = response;
          console.log(`\n捕获到导入 API 调用: ${response.url()}`);
          console.log(`状态码: ${response.status()}`);
          
          try {
            const jsonData = await response.json();
            console.log('API 响应数据:');
            console.log(JSON.stringify(jsonData, null, 2));
            
            // 检查是否包含 context_length 字段
            const responseStr = JSON.stringify(jsonData);
            if (responseStr.includes('context_length')) {
              console.log('\n✓ API 响应中包含 context_length 字段！');
              
              // 提取 context_length 值
              if (jsonData.models || jsonData.data) {
                const models = jsonData.models || jsonData.data;
                models.forEach((model, idx) => {
                  if (model.context_length !== undefined && model.context_length > 0) {
                    console.log(`  模型 ${idx}: ${model.name || model.id} - context_length: ${model.context_length}`);
                  }
                });
              }
            } else {
              console.log('\n✗ API 响应中未找到 context_length 字段');
            }
          } catch (e) {
            console.log('无法解析 JSON 响应:', e.message);
            const textData = await response.text();
            console.log('响应文本(前500字符):', textData.substring(0, 500));
          }
        }
      });
      
      await importButton.click();
      console.log('已点击导入按钮，等待响应...');
      
      // 等待导入完成（最多等待 10 秒）
      await page.waitForTimeout(10000);
      
    } else {
      console.log('未找到导入模型按钮');
      console.log('页面上的所有按钮:');
      const buttons = await page.evaluate(() => {
        return Array.from(document.querySelectorAll('button'))
          .map(btn => ({ text: btn.textContent?.trim(), class: btn.className }));
      });
      console.log(JSON.stringify(buttons, null, 2));
    }
    
    // 等待页面更新
    await page.waitForTimeout(2000);
    
    // 截图 - 导入后状态
    await page.screenshot({ path: 'debug_step5_after_import.png', fullPage: true });
    console.log('\n截图已保存: debug_step5_after_import.png');
    
    // 步骤 6a: 检查 badge 元素数量
    console.log('\n=== 步骤 6a: 检查 badge 元素 ===');
    const badgeCount = await page.evaluate(() => {
      return document.querySelectorAll('.model-context-length-badge').length;
    });
    console.log(`.model-context-length-badge 元素数量: ${badgeCount}`);
    
    // 步骤 6b: 查找任何包含 'badge' 或 'context' 的元素
    console.log('\n=== 步骤 6b: 查找 badge/context 相关元素 ===');
    const badgeElements = await page.evaluate(() => {
      const elements = document.querySelectorAll('*');
      const results = [];
      elements.forEach(el => {
        const className = el.className?.toString() || '';
        if ((className.toLowerCase().includes('badge') || className.toLowerCase().includes('context')) && 
            el.children.length === 0 && el.textContent?.trim()) {
          results.push({
            tag: el.tagName,
            class: className,
            html: el.outerHTML,
            text: el.textContent?.trim()
          });
        }
      });
      return results.slice(0, 20); // 限制数量
    });
    console.log(`找到 ${badgeElements.length} 个包含 badge/context 的元素:`);
    badgeElements.forEach((el, idx) => {
      console.log(`\n元素 ${idx + 1}:`);
      console.log(`  标签: ${el.tag}`);
      console.log(`  类名: ${el.class}`);
      console.log(`  文本: ${el.text}`);
      console.log(`  HTML: ${el.html}`);
    });
    
    // 步骤 6c: 检查模型列表区域的结构
    console.log('\n=== 步骤 6c: 检查模型列表结构 ===');
    const modelListStructure = await page.evaluate(() => {
      // 查找可能包含模型的容器
      const modelContainers = document.querySelectorAll('[class*="model"], [class*="list"], table tbody tr, [role="row"]');
      return Array.from(modelContainers).slice(0, 10).map(container => ({
        tag: container.tagName,
        class: container.className,
        childCount: container.children.length,
        textPreview: container.textContent?.trim().substring(0, 150)
      }));
    });
    console.log('模型列表容器:');
    console.log(JSON.stringify(modelListStructure, null, 2));
    
    // 最终截图
    await page.screenshot({ path: 'debug_final_state.png', fullPage: true });
    console.log('\n最终截图已保存: debug_final_state.png');
    
    // 总结
    console.log('\n' + '='.repeat(60));
    console.log('调试总结');
    console.log('='.repeat(60));
    console.log(`✓ Badge 元素 (.model-context-length-badge) 数量: ${badgeCount}`);
    console.log(`✓ 包含 badge/context 类名的元素数: ${badgeElements.length}`);
    console.log(`✓ 已生成 6 张截图用于分析`);
    console.log('='.repeat(60));
    
  } catch (error) {
    console.error('错误:', error.message);
    await page.screenshot({ path: 'debug_error.png', fullPage: true });
    console.log('错误截图已保存: debug_error.png');
  } finally {
    // 保持浏览器打开一段时间以便查看
    console.log('\n浏览器将在 5 秒后关闭...');
    await page.waitForTimeout(5000);
    await browser.close();
    console.log('调试完成！');
  }
})();
