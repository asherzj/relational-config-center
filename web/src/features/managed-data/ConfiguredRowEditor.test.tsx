import { defaultFieldPolicies } from "../../test/field-policy-fixture";
import { render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import userEvent from "@testing-library/user-event";
import { TestRouter } from "../../test/TestRouter";
import { LeaveProtectionProvider } from "../../components/ui/LeaveProtection";
import { ConfiguredRowEditor } from "./ConfiguredRowEditor";

afterEach(() => vi.unstubAllGlobals());
it("AC-015 fails explicitly on configuration read errors and retries before exposing editor fields", async () => {
  const fetch = vi.fn().mockResolvedValueOnce(new Response(JSON.stringify({error:{code:"internal_error", message:"catalog unavailable"}}),{status:500})).mockResolvedValueOnce(new Response(JSON.stringify(defaultFieldPolicies("items", [{name:"id",type:"uint64",nullable:false}]))));
  vi.stubGlobal("fetch", fetch);
  render(<TestRouter initialEntries={["/"]}><LeaveProtectionProvider><ConfiguredRowEditor open tableName="items" operation="ADD" columns={[{name:"id",type:"uint64",nullable:false}]} autoFillFields={new Set()} onClose={()=>{}} onReview={()=>{}} /></LeaveProtectionProvider></TestRouter>);
  expect(await screen.findByRole("alert")).toBeVisible();
  expect(screen.queryByRole("checkbox",{name:"包含 id"})).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole("button",{name:"重试"}));
  expect(await screen.findByRole("checkbox",{name:"包含 id"})).not.toBeChecked();
  await waitFor(()=>expect(fetch).toHaveBeenCalledTimes(2));
});

it("AC-003/015 uses fresh real metadata when the previous query schema has removed, added or generated fields", async () => {
  const metadata = defaultFieldPolicies("items", [
    {name:"id",type:"uint64",nullable:false},
    {name:"new_field",type:"string",nullable:true},
    {name:"computed",type:"string",nullable:false,generated:true},
  ]);
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify(metadata))));
  const review = vi.fn();
  render(<TestRouter initialEntries={["/"]}><LeaveProtectionProvider><ConfiguredRowEditor open tableName="items" operation="ADD"
    columns={[{name:"id",type:"uint64",nullable:false},{name:"removed",type:"string",nullable:false},{name:"computed",type:"string",nullable:false}]}
    autoFillFields={new Set()} onClose={()=>{}} onReview={review} /></LeaveProtectionProvider></TestRouter>);
  await screen.findByRole("checkbox",{name:"包含 id"});
  expect(screen.queryByRole("checkbox",{name:"包含 removed"})).not.toBeInTheDocument();
  expect(screen.queryByRole("checkbox",{name:"包含 computed"})).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole("checkbox",{name:"包含 new_field"}));
  await userEvent.type(screen.getByRole("textbox",{name:"new_field 值"}),"new value");
  await userEvent.click(screen.getByRole("button",{name:"查看 Change Set"}));
  expect(review.mock.calls[0]?.[0]).toEqual({new_field:"new value"});
});
it("AC-015 reads actual values for newly discovered MODIFY fields instead of manufacturing empty strings", async () => {
  const oldColumns=[{name:"id",type:"uint64" as const,nullable:false},{name:"name",type:"string" as const,nullable:false}];
  const freshColumns=[...oldColumns,{name:"new_default",type:"string" as const,nullable:true},{name:"new_null",type:"string" as const,nullable:true}];
  const fetch=vi.fn(async(input: RequestInfo | URL)=>String(input).includes("table-field-policies")
    ? new Response(JSON.stringify(defaultFieldPolicies("items",freshColumns)))
    : new Response(JSON.stringify({columns:freshColumns,rows:[{id:"1",name:"old",new_default:"database default",new_null:null}],record_versions:["0"],page:{page_number:1,page_size:1,total_count:1,total_pages:1}})));
  vi.stubGlobal("fetch",fetch);
  const review=vi.fn();
  render(<TestRouter initialEntries={["/"]}><LeaveProtectionProvider><ConfiguredRowEditor open tableName="items" operation="MODIFY" columns={oldColumns} original={{id:"1",name:"old"}} autoFillFields={new Set()} onClose={()=>{}} onReview={review}/></LeaveProtectionProvider></TestRouter>);
  expect(await screen.findByRole("textbox",{name:"new_default 值"})).toHaveValue("database default");
  expect(screen.getByRole("checkbox",{name:"new_null 使用 NULL"})).toBeChecked();
  await userEvent.click(screen.getByRole("button",{name:"查看 Change Set"}));
  expect(review.mock.calls[0]?.[0]).toEqual({name:"old",new_default:"database default",new_null:null});
});
it("AC-015 does not create an editor from a missing current row and retries the current-schema read", async () => {
  const freshColumns=[{name:"id",type:"uint64" as const,nullable:false},{name:"new_field",type:"string" as const,nullable:true}];
  let attempts=0;
  vi.stubGlobal("fetch",vi.fn(async(input: RequestInfo | URL)=>{
    if(String(input).includes("table-field-policies"))return new Response(JSON.stringify(defaultFieldPolicies("items",freshColumns)));
    attempts++;
    return new Response(JSON.stringify({columns:freshColumns,rows:attempts===1?[]:[{id:"1",new_field:"actual"}],record_versions:attempts===1?[]:["0"],page:{page_number:1,page_size:1,total_count:attempts===1?0:1,total_pages:attempts===1?0:1}}));
  }));
  render(<TestRouter initialEntries={["/"]}><LeaveProtectionProvider><ConfiguredRowEditor open tableName="items" operation="MODIFY" columns={[{name:"id",type:"uint64",nullable:false}]} original={{id:"1"}} autoFillFields={new Set()} onClose={()=>{}} onReview={()=>{}}/></LeaveProtectionProvider></TestRouter>);
  expect(await screen.findByRole("alert")).toBeVisible();
  expect(screen.queryByRole("textbox",{name:"new_field 值"})).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole("button",{name:"重试"}));
  expect(await screen.findByRole("textbox",{name:"new_field 值"})).toHaveValue("actual");
  expect(attempts).toBe(2);
});
it("AC-015 rejects a second schema change between metadata and the current-row read", async () => {
  const configured=[{name:"id",type:"uint64" as const,nullable:false},{name:"value",type:"string" as const,nullable:true}];
  let attempts=0;
  vi.stubGlobal("fetch",vi.fn(async(input: RequestInfo | URL)=>{
    if(String(input).includes("table-field-policies"))return new Response(JSON.stringify(defaultFieldPolicies("items",configured)));
    attempts++;
    return new Response(JSON.stringify({columns:attempts===1?[configured[0],{name:"value",type:"int64",nullable:false}]:configured,rows:[{id:"1",value:"5"}],record_versions:["0"],page:{page_number:1,page_size:1,total_count:1,total_pages:1}}));
  }));
  render(<TestRouter initialEntries={["/"]}><LeaveProtectionProvider><ConfiguredRowEditor open tableName="items" operation="MODIFY" columns={[{name:"id",type:"uint64",nullable:false}]} original={{id:"1"}} autoFillFields={new Set()} onClose={()=>{}} onReview={()=>{}}/></LeaveProtectionProvider></TestRouter>);
  expect(await screen.findByRole("alert")).toBeVisible();
  expect(screen.queryByRole("textbox",{name:"value 值"})).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole("button",{name:"重试"}));
  expect(await screen.findByRole("textbox",{name:"value 值"})).toHaveValue("5");
});
