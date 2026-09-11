import {render,screen} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {expect,it,vi} from "vitest";
import {ReleasePerson} from "./ReleasePerson";

const accountID="4df747f9-52e7-4507-94fe-af964e9817de";

it("keeps the account ID behind a keyboard-accessible disclosure and copies the complete value",async()=>{
 const user=userEvent.setup();
 const copy=vi.spyOn(navigator.clipboard,"writeText").mockResolvedValue();
 render(<ReleasePerson id={accountID} name="本地管理员"/>);
 expect(screen.queryByText(accountID)).not.toBeInTheDocument();
 await user.tab();
 expect(screen.getByRole("button",{name:"查看本地管理员的账号信息"})).toHaveFocus();
 await user.keyboard("{Enter}");
 expect(screen.getByText(accountID)).toBeVisible();
 await user.click(screen.getByRole("button",{name:"复制账号 ID"}));
 expect(copy).toHaveBeenCalledWith(accountID);
 expect(screen.getByRole("status")).toHaveTextContent("已复制账号 ID");
 await user.click(screen.getByRole("button",{name:"收起本地管理员的账号信息"}));
 expect(screen.queryByText(accountID)).not.toBeInTheDocument();
});

it("keeps an unnamed account identifiable and offers its full ID on demand",async()=>{
 const user=userEvent.setup();
 render(<ReleasePerson id={accountID}/>);
 expect(screen.getByText("账号 4df747f9")).toBeVisible();
 await user.click(screen.getByRole("button",{name:"查看账号 4df747f9的账号信息"}));
 expect(screen.getByText(accountID)).toBeVisible();
});

it("retains the full selectable account ID when clipboard access fails",async()=>{
 const user=userEvent.setup();
 vi.spyOn(navigator.clipboard,"writeText").mockRejectedValue(new Error("Clipboard denied"));
 render(<ReleasePerson id={accountID} name="本地管理员"/>);
 await user.click(screen.getByRole("button",{name:"查看本地管理员的账号信息"}));
 await user.click(screen.getByRole("button",{name:"复制账号 ID"}));
 expect(screen.getByRole("alert")).toHaveTextContent("复制失败，请选择完整账号 ID 手动复制。");
 expect(screen.getByText(accountID)).toBeVisible();
});
