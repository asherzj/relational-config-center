import { render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import userEvent from "@testing-library/user-event";
import { TestRouter } from "../../test/TestRouter";
import { LeaveProtectionProvider } from "../../components/ui/LeaveProtection";
import { ConfiguredRowEditor } from "./ConfiguredRowEditor";

afterEach(() => vi.unstubAllGlobals());
it("AC-015 fails explicitly on configuration read errors and retries before exposing editor fields", async () => {
  const fetch = vi.fn().mockResolvedValueOnce(new Response(JSON.stringify({error:{code:"internal_error", message:"catalog unavailable"}}),{status:500})).mockResolvedValueOnce(new Response(JSON.stringify({table_name:"items",fields:[]})));
  vi.stubGlobal("fetch", fetch);
  render(<TestRouter initialEntries={["/"]}><LeaveProtectionProvider><ConfiguredRowEditor open tableName="items" operation="ADD" columns={[{name:"id",type:"uint64",nullable:false}]} autoFillFields={new Set()} onClose={()=>{}} onReview={()=>{}} /></LeaveProtectionProvider></TestRouter>);
  expect(await screen.findByRole("alert")).toBeVisible();
  expect(screen.queryByRole("checkbox",{name:"包含 id"})).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole("button",{name:"重试"}));
  expect(await screen.findByRole("checkbox",{name:"包含 id"})).not.toBeChecked();
  await waitFor(()=>expect(fetch).toHaveBeenCalledTimes(2));
});
