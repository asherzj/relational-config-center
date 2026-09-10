import {expect,it} from "vitest";
import {rememberReleaseRequest,pendingReleaseRequests} from "./release-journal";

it("persists a request larger than sessionStorage without changing its key or body",async()=>{
 const account="journal-large-account";
 const body=JSON.stringify({title:"large draft",items:[{table_name:"config",operation:"ADD",content:{payload:"x".repeat(9<<20)}}]});
 await rememberReleaseRequest(account,{scope:"create",path:"/api/v1/release-orders",method:"POST",body,key:"large-original-key",label:"large draft"});
 expect(pendingReleaseRequests(account)[0]).toMatchObject({key:"large-original-key",body});
 expect(sessionStorage.length).toBe(0);
});
