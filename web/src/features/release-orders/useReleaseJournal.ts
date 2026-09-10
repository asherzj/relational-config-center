import {useEffect,useState} from "react";
import {useWorkspaceIdentity} from "../accounts/ProtectedWorkspace";
import {ensureReleaseRequests,hydrateReleaseRequests,pendingReleaseRequests,releaseJournalChanged,releaseJournalState} from "./release-journal";

// Storage availability belongs to publication. Account and unrelated read pages
// remain usable while a journal is unavailable; release writes wait for it.
export function useReleaseJournal(){
 const accountID=useWorkspaceIdentity()!.account.id;
 const [,refresh]=useState(0);
 useEffect(()=>{
  const update=()=>refresh(value=>value+1);
  window.addEventListener(releaseJournalChanged,update);
  void ensureReleaseRequests(accountID).catch(()=>{});
  return()=>window.removeEventListener(releaseJournalChanged,update);
 },[accountID]);
 return {...releaseJournalState(accountID),requests:pendingReleaseRequests(accountID),reload:()=>hydrateReleaseRequests(accountID).catch(()=>{})};
}
