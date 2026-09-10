// Browser acceptance follows the normal business action after a reload. These
// helpers never issue a business HTTP request or an independent result query.
const button=(page,name)=>page.getByRole('button',{name,exact:true});
async function repeatReleaseAction(page, action, confirmation) {
  await button(page,action).click();
  if(action==='重新准备')await button(page,'继续重新准备').click();
  await button(page,confirmation).click();
}
async function reopenDraftSave(page) {
  await page.goto(new URL('/configuration/release-orders',page.url()).href);
  await button(page,'新建草稿').click();
}
async function repeatDraftSave(page) {
  await reopenDraftSave(page);
  await button(page,'确认并保存草稿').click();
}
module.exports={repeatReleaseAction,reopenDraftSave,repeatDraftSave};
