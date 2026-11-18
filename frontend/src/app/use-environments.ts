import { useQuery } from "@tanstack/react-query";
import { api } from "../shared/api";
import type { ProjectDetail } from "../shared/session";
import { useAuth } from "../shared/session";

export type ScopeOption = { value: string; label: string };

/** Loads the current project's environments as choices, keeping an unknown selected value visible. */
export function useEnvironmentOptions(environment: string) {
  const { credential, session } = useAuth();
  const projectID = session?.project?.project.id;
  const detail = useQuery({
    queryKey: ["project", projectID],
    enabled: Boolean(projectID) && !session?.user.must_change_password,
    queryFn: ({ signal }) => api<ProjectDetail>(`/api/v1/projects/${projectID}`, credential, { signal }),
  });
  const environments = detail.data?.environments ?? [];
  const options: ScopeOption[] = [
    { value: "", label: "Default environment" },
    ...(environment && !environments.some((item) => item.name === environment || item.id === environment)
      ? [{ value: environment, label: environment }] : []),
    ...environments.map((item) => ({ value: environment === item.id ? item.id : item.name, label: item.name })),
  ];
  return { detail, options, projectID };
}

export const initialOf = (name?: string) => (name?.trim().charAt(0) || "U").toUpperCase();
