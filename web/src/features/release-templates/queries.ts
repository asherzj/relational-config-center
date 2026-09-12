import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createReleaseTemplate, deleteReleaseTemplate, getReleaseTemplate, listReleaseTemplates, replaceReleaseTemplate, setReleaseTemplateEnabled } from "../../api/release-templates";
import { shouldRetryQuery } from "../../api/client";
import type { ReleaseTemplateDraft } from "./model";

export const releaseTemplateKeys = { list: ["release-templates"] as const, detail: (code: string) => ["release-templates", code] as const };
export const useReleaseTemplates = () => useQuery({ queryKey: releaseTemplateKeys.list, queryFn: listReleaseTemplates, retry: shouldRetryQuery });
export const useReleaseTemplate = (code?: string) => useQuery({ queryKey: releaseTemplateKeys.detail(code ?? ""), queryFn: () => getReleaseTemplate(code!), enabled: Boolean(code), retry: shouldRetryQuery });
function useTemplateMutation<T>(run: (input: T) => Promise<unknown>) {
  const client = useQueryClient();
  return useMutation({ mutationFn: run, onSuccess: () => client.invalidateQueries({ queryKey: releaseTemplateKeys.list }) });
}
export const useCreateReleaseTemplate = () => useTemplateMutation(({ draft, key }: { draft: ReleaseTemplateDraft; key: string }) => createReleaseTemplate(draft, key));
export const useReplaceReleaseTemplate = () => useTemplateMutation(({ code, draft, version, key }: { code: string; draft: ReleaseTemplateDraft; version: string; key: string }) => replaceReleaseTemplate(code, draft, version, key));
export const useSetReleaseTemplateEnabled = () => useTemplateMutation(({ code, enabled, version, key }: { code: string; enabled: boolean; version: string; key: string }) => setReleaseTemplateEnabled(code, enabled, version, key));
export const useDeleteReleaseTemplate = () => useTemplateMutation(({ code, version, key }: { code: string; version: string; key: string }) => deleteReleaseTemplate(code, version, key));
