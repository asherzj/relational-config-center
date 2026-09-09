export function ReleaseTime({value}:{value:string}){
 const date=new Date(value);
 return <time dateTime={value} title={value} className="break-words">{Number.isNaN(date.getTime())?value:date.toLocaleString("zh-CN",{year:"numeric",month:"2-digit",day:"2-digit",hour:"2-digit",minute:"2-digit",second:"2-digit",hour12:false})}</time>;
}
