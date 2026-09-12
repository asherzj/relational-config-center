const fs = require('node:fs/promises');
const assert = require('node:assert/strict');
const playwright = require(process.env.RCC_PLAYWRIGHT_MODULE || 'playwright');
const { browserOptions, registerFixtureAccount } = require('./local-account.cjs');
const base = process.env.RCC_WEB_URL;
const output = process.env.RCC_E2E_OUTPUT;
const engineName = process.env.RCC_E2E_ENGINE;
const table = process.env.RCC_E2E_TABLE;
(async () => {
  const attempts=[];let failure=null;
  const browser=await playwright[engineName].launch(browserOptions());
  const context=await browser.newContext({viewport:{width:320,height:568}});
  await registerFixtureAccount(context,base);
  const page=await context.newPage();
  await page.addInitScript(()=>{
    window.__selectionEvents=[];
    for(const type of ['input','change']) document.addEventListener(type,e=>{
      if(e.target.getAttribute('aria-label')!=='Managed Table')return;
      window.__selectionEvents.push({type,value:e.target.value,at:performance.now()});
      setTimeout(()=>window.__selectionEvents.push({type:'later-'+type,value:e.target.value,at:performance.now()}),0);
    },true);
  });
  try{
    for(let iteration=0;iteration<20;iteration++){
      const requests=[];
      const listener=req=>{if(req.url().includes('/tables/')&&req.url().endsWith('/query'))requests.push(req.url());};
      page.on('request',listener);
      await page.goto(base+'/configuration/managed-data');
      const select=page.getByRole('combobox',{name:'Managed Table',exact:true});
      const selected=await select.selectOption(table);
      await page.getByRole('button',{name:'新增记录',exact:true}).waitFor();
      await page.getByRole('button',{name:'新增记录',exact:true}).click();
      const dialog=page.getByRole('dialog',{name:/^新增 .* 记录$/});await dialog.waitFor();
      const title=await dialog.getByRole('heading').innerText();
      attempts.push({iteration,selected,value:await select.inputValue(),title,requests,events:await page.evaluate(()=>window.__selectionEvents)});
      page.off('request',listener);
      if(title!==`新增 ${table} 记录`){await page.screenshot({path:output+'/wrong-table.png',fullPage:true});assert.equal(title,`新增 ${table} 记录`);}
      await dialog.getByRole('button',{name:'关闭',exact:true}).click();
    }
  }catch(error){failure={name:error.name,message:error.message};process.exitCode=1;}
  finally{await browser.close();await fs.writeFile(output+'/result.json',JSON.stringify({ok:!failure,engine:engineName,attempts,failure},null,2));console.log(JSON.stringify({iterations:attempts.length,failure}));}
})().catch(error=>{console.error(error);process.exitCode=1;});
