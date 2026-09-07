import {useState} from "react";
import {Button} from "../../components/ui/Button";
import {Input} from "../../components/shadcn/input";
export const releasePageSize=20;
export function ReleaseItemPager({count,page,onPage,onLocate,label="明细"}:{count:number;page:number;onPage:(page:number)=>void;onLocate?:(index:number)=>void;label?:string}){
 const [jump,setJump]=useState("");
 if(count<=releasePageSize)return <p className="mb-3">共 {count} 项{label}</p>;
 return <nav aria-label={`${label}分页`} className="my-4 flex flex-wrap items-center gap-3"><p>共 {count.toLocaleString("en-US")} 项，当前展示 {page*releasePageSize+1}–{Math.min((page+1)*releasePageSize,count)} 项</p><Button disabled={page===0} onClick={()=>onPage(page-1)}>上一页{label}</Button><Button disabled={(page+1)*releasePageSize>=count} onClick={()=>onPage(page+1)}>下一页{label}</Button><label>定位{label}<Input aria-label={`定位${label}`} className="w-28" inputMode="numeric" value={jump} placeholder={`1–${count}`} onChange={event=>{const value=event.target.value;setJump(value);const index=Number(value);if(Number.isInteger(index)&&index>=1&&index<=count)onLocate?onLocate(index-1):onPage(Math.floor((index-1)/releasePageSize))}}/></label></nav>;
}
