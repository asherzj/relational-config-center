const {webkit}=require('/workspace/web/node_modules/playwright');
(async()=>{
 const context=await webkit.launchPersistentContext('/evidence/profile',{headless:true});
 const page=await context.newPage();
 await page.setContent('<button onclick="this.textContent=\'done\'">go</button>');
 await page.getByRole('button',{name:'go'}).click();
 if(await page.getByRole('button').textContent()!=='done')throw Error('click failed');
 await context.close();
 const reopened=await webkit.launchPersistentContext('/evidence/profile',{headless:true});
 await reopened.close();
 process.stdout.write('real WebKit persistent launch/click/reopen PASS\n');
})().catch(e=>{console.error(e);process.exitCode=1});
