import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import {
  usePluginSettingsDetail,
  usePluginSettingsList,
  useUpdatePluginSettings,
} from "@/hooks/queries/pluginSettings";
import { pluginRouteHref } from "@/lib/pluginRouteHref";
import { navigateToPluginRoute } from "@/lib/buildPluginHref";

import { Save } from "lucide-react";
function PluginSettingsCard({ installationId }: { installationId: number }) {
  const { data, isLoading } = usePluginSettingsDetail(
    installationId,
    installationId > 0,
  );
  const updateSettings = useUpdatePluginSettings();
  const [values, setValues] = useState<Record<string, string>>({});

  if (isLoading || !data) {
    return <Skeleton className="h-40 w-full rounded-xl" />;
  }

  const userRoutes = data.installation.routes.filter(
    (route) => route.navigable && route.navigation_kind === "user",
  );
  const mergedValues = { ...data.values, ...values };

  return (
    <section className="surface-panel-subtle space-y-4 rounded-[1.25rem] p-5">
      <div className="space-y-1">
        <h3 className="text-base font-semibold">
          {data.installation.plugin_id}
        </h3>
        <p className="text-muted-foreground text-xs">
          {data.installation.version}
        </p>
      </div>

      <div className="grid gap-4">
        {data.installation.user_config_schema.map((schema) => (
          <div key={schema.key} className="space-y-2">
            <Label htmlFor={`${installationId}-${schema.key}`}>
              {schema.title || schema.key}
            </Label>
            <Input
              id={`${installationId}-${schema.key}`}
              value={mergedValues[schema.key] ?? ""}
              onChange={(event) =>
                setValues((current) => ({
                  ...current,
                  [schema.key]: event.target.value,
                }))
              }
            />
          </div>
        ))}
      </div>

      <div className="flex flex-wrap gap-2">
        <Button
          onClick={() =>
            updateSettings.mutate({
              id: installationId,
              body: { values: mergedValues },
            })
          }
        >
          <Save />
          Save
        </Button>
        {userRoutes.map((route) => {
          const href = pluginRouteHref(installationId, route.path);
          return (
            <a
              key={route.id}
              href={href}
              onClick={(e) => {
                e.preventDefault();
                void navigateToPluginRoute(href);
              }}
              className="inline-flex items-center rounded-md border px-3 py-2 text-sm"
            >
              {route.navigation_label || route.path}
            </a>
          );
        })}
      </div>
    </section>
  );
}

export default function PluginSettings() {
  const { data, isLoading } = usePluginSettingsList();

  if (isLoading) {
    return (
      <div className="space-y-6">
        <div className="space-y-2">
          <Skeleton className="h-8 w-32" />
          <Skeleton className="h-4 w-64" />
        </div>
        <div className="space-y-4">
          {Array.from({ length: 2 }).map((_, i) => (
            <Skeleton key={i} className="h-40 w-full rounded-xl" />
          ))}
        </div>
      </div>
    );
  }

  const installations = data?.installations ?? [];

  return (
    <div className="space-y-6">
      <div className="space-y-2">
        <h2 className="text-2xl font-semibold tracking-tight">Plugins</h2>
        <p className="text-muted-foreground text-sm">
          Configure installed plugins and open plugin-hosted account pages.
        </p>
      </div>

      <div className="space-y-4">
        {installations.length === 0 ? (
          <div className="text-muted-foreground rounded-[1rem] border p-5 text-sm">
            No plugins expose user settings for this account.
          </div>
        ) : (
          installations.map((installation) => (
            <PluginSettingsCard
              key={installation.id}
              installationId={installation.id}
            />
          ))
        )}
      </div>
    </div>
  );
}
