import {useState} from "react";
import {useQuery, useQueryClient} from "@tanstack/react-query";
import {ApiError, shouldRetryQuery} from "../../api/client";
import {releaseOrders, type ReleaseHeader} from "../../api/release-orders";
import {ErrorState, LoadingState} from "../../components/ui/Feedback";
import {ReleaseDiff} from "./ReleaseDiff";
import {PublicationResult} from "./PublicationResult";
import {ReleaseItemPager, releasePageSize} from "./ReleaseItemPager";

type Props = {
  order: ReleaseHeader;
  kind?: "request" | "PUBLICATION" | "ROLLBACK";
  people?: Record<string, string>;
};

export function PagedReleaseDetails({order, kind = "request", people = {}}: Props) {
  const client = useQueryClient();
  const [requestedPage, setPage] = useState(0);
  const [located, setLocated] = useState<number>();
  const lastPage = Math.max(0, Math.ceil(order.item_count / releasePageSize) - 1);
  const page = Math.min(requestedPage, lastPage);
  const offset = page * releasePageSize;
  const details = useQuery({
    queryKey: ["release-details", order.id, order.version, offset],
    queryFn: () => releaseOrders.details(order, offset, releasePageSize),
    retry: shouldRetryQuery,
  });
  const execution = order.executions.find(execution => execution.kind === kind);
  const stale = details.error instanceof ApiError && details.error.code === "release_version_conflict";
  const retry = () => {
    if (stale) void client.invalidateQueries({queryKey: ["release-order", order.id]});
    else void details.refetch();
  };

  let content;
  if (details.isPending) content = <LoadingState/>;
  else if (details.isError) content = <ErrorState error={details.error} onRetry={retry}/>;
  else if (kind === "request") {
    content = <ReleaseDiff key={`${order.version}:${offset}:${located}`} order={details.data}
      offset={offset} paged operationCounts={order.operation_counts} initiallyExpanded={located ?? offset}/>;
  } else if (execution) {
    const commands = details.data.items.map(item => kind === "PUBLICATION" ? item.publication! : item.rollback!);
    content = <PublicationResult result={execution} commands={commands} offset={offset}
      people={people} restoration={kind === "ROLLBACK"}/>;
  } else content = <p role="alert">尚无实际数据库执行结果。</p>;

  return <>
    <ReleaseItemPager count={order.item_count} page={page} label={kind === "request" ? "明细" : "结果"}
      onPage={next => {setPage(next); setLocated(undefined);}}
      onLocate={index => {setPage(Math.floor(index / releasePageSize)); setLocated(index);}}/>
    {content}
  </>;
}
