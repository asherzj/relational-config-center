export function ReleasePerson({id,name}:{id:string;name?:string}){
 const label=name?.trim()||id;
 const initial=Array.from(label)[0]??"?";
 return <span className="inline-flex min-w-0 items-center gap-2 align-middle">
  <span aria-hidden className="flex size-7 shrink-0 items-center justify-center rounded-full bg-muted text-xs font-semibold text-muted-foreground">{initial}</span>
  <span className="min-w-0"><strong className="block break-all font-medium text-foreground">{label}</strong><code className="block break-all text-xs text-muted-foreground">{id}</code></span>
 </span>;
}
