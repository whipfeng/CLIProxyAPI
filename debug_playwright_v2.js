const { chromium } = require('playwright');

(async () => {
  const browser = await chromium.launch({ headless: false, slowMo: 500 });
  const page = await browser.newPage();
  
  console.log('=== 开始调试 Context Length Badge 问题 (增强版) ===\n');
  
  try {
    // 步骤 1: 导航到管理页面
    console.log('步骤 1: 导航到 http://localhost:8317/management.html');
    await page.goto('http://localhost:8317/management.html', { waitUntil: 'networkidle', timeout: 30000 });
    console.log('等待页面完全加载...');
    await page.waitForTimeout(3000);
    
    // 截图 - 初始状态
    await page.screenshot({ path: 'debug2_step1_initial.png', fullPage: true });
    console.log('截图已保存: debug2_step1_initial.png\n');
    
    // 详细检查登录表单
    console.log('步骤 2: 详细检查登录表单...');
    const formInfo = await page.evaluate(() => {
      const inputs = Array.from(document.querySelectorAll('input'));
      const buttons = Array.from(document.querySelectorAll('button'));
      return {
        inputCount: inputs.length,
        inputs: inputs.map(inp => ({
          type: inp.type,
          name: inp.name,
          id: inp.id,
          placeholder: inp.placeholder,
          className: inp.className,
          visible: inp.offsetParent !== null
        })),
        buttonCount: buttons.length,
        buttons: buttons.map(btn => ({
          text: btn.textContent?.trim(),
          type: btn.type,
          className: btn.className,
          visible: btn.offsetParent !== null
        })),
        url: window.location.href,
        hasLoginForm: !!document.querySelector('form')
      };
    });
    
    console.log('表单信息:');
    console.log(`  - 输入框数量: ${formInfo.inputCount}`);
    console.log(`  - 按钮数量: ${formInfo.buttonCount}`);
    console.log(`  - 当前URL: ${formInfo.url}`);
    console.log(`  - 有表单: ${formInfo.hasLoginForm}`);
    console.log('\n输入框详情:');
    formInfo.inputs.forEach((inp, idx) => {
      console.log(`  ${idx + 1}. type=${inp.type}, name=${inp.name}, id=${inp.id}, placeholder=${inp.placeholder}, class=${inp.className}, visible=${inp.visible}`);
    });
    console.log('\n按钮详情:');
    formInfo.buttons.forEach((btn, idx) => {
      console.log(`  ${idx + 1}. text="${btn.text}", type=${btn.type}, class=${btn.class}, visible=${btn.visible}`);
    });
    
    // 尝试登录
    console.log('\n尝试使用管理密钥 sk-admin-key 登录...');
    
    // 查找密码输入框（优先查找可见的）
    const passwordInput = await page.$('input[type="password"]:visible, input[name*="key"]:visible, input[placeholder*="密钥"]:visible, input[placeholder*="password"]:visible, input[type="password"], input[type="text"]');
    
    if (passwordInput) {
      console.log('找到输入框，清空并输入密钥...');
      
      // 先点击输入框确保聚焦
      await passwordInput.click();
      await page.waitForTimeout(500);
      
      // 清空现有内容
      await passwordInput.fill('');
      await page.waitForTimeout(300);
      
      // 输入正确的管理密钥
      await passwordInput.type('sk-admin-key', { delay: 100 });
      console.log('已输入管理密钥');
      
      // 验证输入值
      const inputValue = await passwordInput.inputValue();
      console.log(`输入框当前值: ${inputValue}`);
      
      await page.waitForTimeout(1000);
      
      // 截图 - 输入后
      await page.screenshot({ path: 'debug2_step2_input_filled.png' });
      console.log('截图: debug2_step2_input_filled.png');
      
      // 查找并点击提交按钮
      console.log('\n查找登录按钮...');
      const submitButton = await page.$('button[type="submit"]:visible, button.btn-primary:visible, button:has-text("登录"):visible, button:has-text("Login"):visible, button:visible');
      
      if (submitButton) {
        const buttonText = await submitButton.textContent();
        console.log(`找到按钮: "${buttonText.trim()}"`);
        
        // 点击按钮
        await submitButton.click();
        console.log('已点击登录按钮');
        
        // 等待导航或内容变化
        console.log('等待页面响应...');
        
        try {
          // 等待 URL 变化或新内容加载
          await page.waitForNavigation({ timeout: 10000 }).catch(() => {
            console.log('等待导航超时，继续...');
          });
        } catch (e) {
          console.log('导航等待异常:', e.message);
        }
        
        await page.waitForTimeout(3000);
        
        // 截图 - 登录后
        await page.screenshot({ path: 'debug2_step3_after_login.png', fullPage: true });
        console.log('截图: debug2_step3_after_login.png');
        
        // 检查是否成功登录
        const currentUrl = page.url();
        console.log(`\n当前URL: ${currentUrl}`);
        
        const stillOnLoginPage = await page.evaluate(() => {
          return !!document.querySelector('input[type="password"]') || 
                 document.body.innerText.includes('管理密钥') ||
                 document.body.innerText.includes('Management Key') ||
                 document.body.innerText.includes('登录');
        });
        
        if (stillOnLoginPage) {
          console.log('⚠️ 似乎仍在登录页面');
          
          // 检查是否有错误消息
          const errorMessages = await page.evaluate(() => {
            const elements = document.querySelectorAll('.error, .alert, [class*="error"], [class*="alert"], [role="alert"]');
            return Array.from(elements).map(el => el.textContent?.trim()).filter(t => t);
          });
          
          if (errorMessages.length > 0) {
            console.log('发现错误消息:');
            errorMessages.forEach(msg => console.log(`  - ${msg}`));
          }
        } else {
          console.log('✓ 登录成功！');
        }
        
      } else {
        console.log('未找到登录按钮');
      }
    } else {
      console.log('未找到密码输入框');
    }
    
    // 步骤 3: 如果成功登录，查找 AI Providers
    console.log('\n=== 步骤 4: 分析页面结构 ===');
    
    const pageStructure = await page.evaluate(() => {
      // 获取所有主要可交互元素
      const allInteractive = Array.from(document.querySelectorAll('a, button, [role="tab"], [role="button"], [onclick], nav a, sidebar a, menu a'));
      
      return {
        url: window.location.href,
        title: document.title,
        interactiveElements: allInteractive
          .filter(el => el.offsetParent !== null)
          .slice(0, 50)
          .map(el => ({
            tag: el.tagName,
            text: el.textContent?.trim().substring(0, 80),
            href: el.href || el.getAttribute('href'),
            className: el.className?.toString().substring(0, 80),
            role: el.getAttribute('role'),
            onclick: el.getAttribute('onclick')
          }))
      };
    });
    
    console.log('页面结构分析:');
    console.log(`标题: ${pageStructure.title}`);
    console.log(`URL: ${pageStructure.url}`);
    console.log(`\n可交互元素 (${pageStructure.interactiveElements.length}个):`);
    pageStructure.interactiveElements.forEach((el, idx) => {
      console.log(`${idx + 1}. [${el.tag}] "${el.text}" | class: ${el.className} | role: ${el.role} | href: ${el.href}`);
    });
    
    // 查找包含 "Provider" 或 "提供商" 或 "AI" 的链接/按钮
    console.log('\n查找 Provider 相关元素...');
    const providerElements = pageStructure.interactiveElements.filter(el => 
      el.text.toLowerCase().includes('provider') || 
      el.text.includes('提供商') ||
      (el.text.toLowerCase().includes('ai') && !el.text.toLowerCase().includes('main'))
    );
    
    if (providerElements.length > 0) {
      console.log('找到 Provider 元素:');
      providerElements.forEach((el, idx) => {
        console.log(`${idx + 1}. [${el.tag}] "${el.text}"`);
      });
      
      // 点击第一个 provider 相关元素
      console.log('\n点击第一个 Provider 元素...');
      const firstProvider = await page.$(`a:text-is("${providerElements[0].text}"), button:text-is("${providerElements[0].text}")`);
      if (firstProvider) {
        await firstProvider.click();
        await page.waitForTimeout(2000);
        await page.screenshot({ path: 'debug2_step5_providers_page.png', fullPage: true });
        console.log('截图: debug2_step5_providers_page.png');
      }
    }
    
    // 步骤 5: 查找 Trae provider
    console.log('\n=== 步骤 5: 查找 Trae Provider ===');
    
    const traeRelated = await page.evaluate(() => {
      const allElements = document.querySelectorAll('*');
      return Array.from(allElements)
        .filter(el => 
          (el.textContent?.toLowerCase().includes('trae') && el.children.length < 10) &&
          el.offsetParent !== null
        )
        .slice(0, 20)
        .map(el => ({
          tag: el.tagName,
          text: el.textContent?.trim().substring(0, 100),
          className: el.className?.toString(),
          id: el.id
        }));
    });
    
    console.log(`找到 ${traeRelated.length} 个与 Trae 相关的元素:`);
    traeRelated.forEach((el, idx) => {
      console.log(`${idx + 1}. [${el.tag}] "${el.text}" | class: ${el.className}`);
    });
    
    if (traeRelated.length > 0) {
      // 尝试点击 Trae 相关元素或其父级的编辑按钮
      console.log('\n尝试进入 Trae 编辑页面...');
      
      // 查找编辑按钮
      let editClicked = false;
      
      for (const traeEl of traeRelated.slice(0, 5)) {
        // 在该元素附近查找编辑按钮
        const editBtn = await page.$(`${traeEl.tag}:has-text("${traeEl.text.substring(0, 20)}") ~ button:has-text("编辑"), ${traeEl.tag}:has-text("${traeEl.text.substring(0, 20)}") button:has-text("Edit"), ${traeEl.tag}:has-text("${traeEl.text.substring(0, 20)}") [class*="edit"]`);
        
        if (editBtn) {
          console.log('找到编辑按钮，点击中...');
          await editBtn.click();
          editClicked = true;
          break;
        }
      }
      
      if (!editClicked && traeRelated.length > 0) {
        // 直接点击第一个 Trae 元素
        console.log('直接点击 Trae 元素...');
        const firstTrae = await page.$(`*:has-text("${traeRelated[0].text.substring(0, 30)}")`);
        if (firstTrae) {
          await firstTrae.click();
          editClicked = true;
        }
      }
      
      if (editClicked) {
        await page.waitForTimeout(2000);
        await page.screenshot({ path: 'debug2_step6_trae_edit.png', fullPage: true });
        console.log('截图: debug2_step6_trae_edit.png');
      }
    }
    
    // 步骤 6: 查找导入模型按钮
    console.log('\n=== 步骤 6: 查找导入模型功能 ===');
    
    const importButtons = await page.evaluate(() => {
      const buttons = document.querySelectorAll('button, a[href*="import"], [class*="import"]');
      return Array.from(buttons)
        .filter(btn => btn.offsetParent !== null)
        .map(btn => ({
          tag: btn.tagName,
          text: btn.textContent?.trim(),
          className: btn.className,
          href: btn.href || btn.getAttribute('href')
        }))
        .filter(btn => 
          btn.text.includes('导入') || 
          btn.text.includes('Import') ||
          btn.className?.toString().toLowerCase().includes('import') ||
          btn.href?.includes('import')
        );
    });
    
    console.log(`找到 ${importButtons.length} 个导入相关按钮:`);
    importButtons.forEach((btn, idx) => {
      console.log(`${idx + 1}. [${btn.tag}] "${btn.text}" | class: ${btn.className}`);
    });
    
    if (importButtons.length > 0) {
      console.log('\n准备点击导入按钮并监听 API 响应...');
      
      // 设置网络监听
      const apiResponses = [];
      
      page.on('response', async (response) => {
        const url = response.url();
        if (url.includes('/api/') || url.includes('/import')) {
          console.log(`\n捕获到 API 调用:`);
          console.log(`  URL: ${url}`);
          console.log(`  状态: ${response.status()}`);
          
          try {
            const contentType = response.headers()['content-type'] || '';
            if (contentType.includes('json')) {
              const json = await response.json();
              console.log(`  响应类型: JSON`);
              
              // 存储响应以便后续分析
              apiResponses.push({
                url: url,
                status: response.status(),
                data: json
              });
              
              // 特别检查 context_length
              const responseStr = JSON.stringify(json);
              if (responseStr.includes('context_length')) {
                console.log(`  ⚠️ 发现 context_length 字段！`);
                
                // 提取详细信息
                if (json.models || json.data) {
                  const models = json.models || json.data;
                  models.forEach((model, idx) => {
                    if (model.context_length !== undefined) {
                      console.log(`    模型 ${idx}: ${model.name || model.id || 'unknown'} - context_length: ${model.context_length}`);
                    }
                  });
                }
              }
            } else {
              const text = await response.text();
              console.log(`  响应类型: ${contentType}`);
              console.log(`  内容预览: ${text.substring(0, 300)}`);
            }
          } catch (e) {
            console.log(`  解析响应失败: ${e.message}`);
          }
        }
      });
      
      // 点击导入按钮
      const importBtn = await page.$(`button:has-text("${importButtons[0].text}"), a:has-text("${importButtons[0].text}")`);
      if (importBtn) {
        console.log(`\n点击按钮: "${importButtons[0].text}"`);
        await importBtn.click();
        
        // 等待导入完成
        console.log('等待导入完成 (最多15秒)...');
        await page.waitForTimeout(15000);
        
        // 截图 - 导入后
        await page.screenshot({ path: 'debug2_step7_after_import.png', fullPage: true });
        console.log('\n截图: debug2_step7_after_import.png');
      }
    }
    
    // 最终检查
    console.log('\n' + '='.repeat(70));
    console.log('最终检查结果');
    console.log('='.repeat(70));
    
    // 6a. 检查 badge 元素
    const badgeCount = await page.evaluate(() => {
      return document.querySelectorAll('.model-context-length-badge').length;
    });
    console.log(`\n6a. .model-context-length-badge 元素数量: ${badgeCount}`);
    
    // 6b. 查找所有包含 badge/context 的元素
    const allBadgeElements = await page.evaluate(() => {
      const elements = document.querySelectorAll('*');
      return Array.from(elements).filter(el => {
        const cls = el.className?.toString() || '';
        return (cls.toLowerCase().includes('badge') || cls.toLowerCase().includes('context-length')) && 
               el.children.length === 0 && 
               el.textContent?.trim();
      }).map(el => ({
        tag: el.tagName,
        class: el.className,
        text: el.textContent?.trim(),
        html: el.outerHTML.substring(0, 200)
      }));
    });
    console.log(`\n6b. 包含 badge/context-length 类名的元素数量: ${allBadgeElements.length}`);
    if (allBadgeElements.length > 0) {
      allBadgeElements.forEach((el, idx) => {
        console.log(`\n  元素 ${idx + 1}:`);
        console.log(`    标签: ${el.tag}`);
        console.log(`    类名: ${el.class}`);
        console.log(`    文本: ${el.text}`);
        console.log(`    HTML: ${el.html}`);
      });
    } else {
      console.log('  未找到任何 badge/context-length 元素');
    }
    
    // 6c. 检查模型列表区域
    const modelListInfo = await page.evaluate(() => {
      // 查找可能的模型列表容器
      const selectors = [
        '[class*="model-list"]',
        '[class*="modelList"]',
        'table',
        '[class*="table"]',
        '[role="grid"]',
        '[role="listbox"]',
        '[class*="list"]'
      ];
      
      const containers = [];
      selectors.forEach(sel => {
        const els = document.querySelectorAll(sel);
        els.forEach(el => {
          if (el.offsetParent !== null) {
            containers.push({
              selector: sel,
              tag: el.tagName,
              class: el.className?.toString().substring(0, 60),
              childCount: el.children.length,
              rowCount: el.querySelectorAll('tr, [role="row"]').length,
              textPreview: el.textContent?.trim().substring(0, 200)
            });
          }
        });
      });
      
      return containers.slice(0, 10);
    });
    
    console.log(`\n6c. 找到的模型列表容器 (${modelListInfo.length}个):`);
    modelListInfo.forEach((container, idx) => {
      console.log(`\n  容器 ${idx + 1}:`);
      console.log(`    选择器: ${container.selector}`);
      console.log(`    标签: ${container.tag}`);
      console.log(`    类名: ${container.class}`);
      console.log(`    子元素数: ${container.childCount}`);
      console.log(`    行数: ${container.rowCount}`);
      console.log(`    文本预览: ${container.textPreview?.substring(0, 100)}...`);
    });
    
    // 最终截图
    await page.screenshot({ path: 'debug2_final_state.png', fullPage: true });
    console.log('\n最终截图: debug2_final_state.png');
    
    console.log('\n' + '='.repeat(70));
    console.log('总结');
    console.log('='.repeat(70));
    console.log(`✓ Badge 元素 (.model-context-length-badge): ${badgeCount} 个`);
    console.log(`✓ 包含 badge/context 类名的元素: ${allBadgeElements.length} 个`);
    console.log(`✓ API 响应记录: ${apiResponses.length} 条`);
    console.log(`✓ 生成的截图: 7+ 张`);
    console.log('='.repeat(70));
    
    // 保持浏览器打开一段时间
    console.log('\n浏览器将在 10 秒后关闭（可以查看当前状态）...');
    await page.waitForTimeout(10000);
    
  } catch (error) {
    console.error('\n发生错误:', error.message);
    console.error(error.stack);
    await page.screenshot({ path: 'debug2_error.png', fullPage: true });
    console.log('错误截图: debug2_error.png');
  } finally {
    await browser.close();
    console.log('\n调试完成！');
  }
})();
