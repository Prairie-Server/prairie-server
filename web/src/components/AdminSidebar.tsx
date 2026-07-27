import { Link, useLocation } from "react-router";
import { ArrowLeft } from "lucide-react";
import type { ReactNode } from "react";
import { SideNavItem, SideNavSection } from "@/components/SideNav";
import { PrairieBrand } from "@/components/PrairieBrand";
import {
  buildAdminNavSections,
  buildAdminPluginNavItems,
  type AdminNavGroup,
  type AdminNavItem,
} from "@/lib/adminNavigation";
import { navigateToPluginRoute } from "@/lib/buildPluginHref";
import { useAdminPluginInstallations } from "@/hooks/queries/admin/plugins";
import { usePolicyCapability } from "@/hooks/queries/admin/policy";
import { useAdminSessions } from "@/hooks/queries/admin/stats";
import { useBuildInfo } from "@/hooks/queries/admin/system";

interface SidebarItem extends AdminNavItem {
  badge?: ReactNode;
}

interface SidebarSection extends Omit<AdminNavGroup, "items"> {
  label: string;
  items: SidebarItem[];
}

interface AdminSidebarProps {
  onNavigate?: () => void;
}

function useSessionCount() {
  const { data: sessions = [] } = useAdminSessions();
  return sessions.length;
}

export default function AdminSidebar({ onNavigate }: AdminSidebarProps) {
  const location = useLocation();
  const sessionCount = useSessionCount();
  const buildInfo = useBuildInfo();
  const policyCapability = usePolicyCapability();
  // Falls back to "dev build" when the binary carries no VCS/ldflags revision
  // (e.g. `go run` or an image built without BUILD_REVISION) rather than a stark
  // "unavailable".
  let buildDisplay = "dev build";
  if (buildInfo.isPending && !buildInfo.data) {
    buildDisplay = "loading...";
  } else if (buildInfo.isError) {
    buildDisplay = "load failed";
  } else if (buildInfo.data?.available) {
    buildDisplay = buildInfo.data.display;
  }

  const activityBadge =
    sessionCount > 0 ? <span className="live-badge">{sessionCount} live</span> : undefined;
  const sections: SidebarSection[] = buildAdminNavSections({
    policyEditorAvailable: policyCapability.data?.editor_available === true,
  }).map((section) => ({
    ...section,
    items: section.items.map((item) =>
      item.href === "/admin/activity" ? { ...item, badge: activityBadge } : item,
    ),
  }));

  // Use the admin installations endpoint, not /settings/plugins — the user
  // settings endpoint filters to plugins that expose user settings / a user-
  // navigable route, which excludes admin-only plugins like arrproxy and
  // arrouter. The admin sidebar needs the full installation list to render
  // its "Plugin Apps" group.
  const { data: adminInstallations } = useAdminPluginInstallations();
  const adminPluginItems = buildAdminPluginNavItems(adminInstallations);
  if (adminPluginItems.length > 0) {
    sections.push({ label: "Plugin Apps", items: adminPluginItems });
  }

  function isActive(item: SidebarItem) {
    if (item.exact) return location.pathname === item.href;
    return location.pathname === item.href || location.pathname.startsWith(`${item.href}/`);
  }

  return (
    <aside className="border-sidebar-border/70 bg-sidebar/92 fixed top-0 bottom-0 left-0 z-40 flex w-[240px] flex-col border-r backdrop-blur-2xl">
      {/* Logo */}
      <div className="flex items-center gap-2.5 px-5 pt-6 pb-4">
        <Link
          to="/"
          onClick={onNavigate}
          aria-label="Go to app home"
          className="focus-visible:ring-ring/50 inline-flex rounded-md transition-opacity hover:opacity-85 focus-visible:ring-[3px] focus-visible:outline-none"
        >
          <PrairieBrand className="brand-reveal h-14 w-[132px]" />
        </Link>
      </div>
      {/* Nav sections */}
      <nav
        aria-label="Admin navigation"
        className="sidebar-scroll flex-1 space-y-5 overflow-y-auto px-3"
      >
        {sections.map((section) => (
          <SideNavSection key={section.label} label={section.label} idPrefix="admin-nav">
            {section.items.map((item) =>
              item.external ? (
                <SideNavItem
                  key={item.href}
                  label={item.label}
                  icon={item.icon}
                  href={item.href}
                  external
                  active={isActive(item)}
                  badge={item.badge}
                  onClick={(e) => {
                    e.preventDefault();
                    void navigateToPluginRoute(item.href);
                    onNavigate?.();
                  }}
                />
              ) : (
                <SideNavItem
                  key={item.href}
                  label={item.label}
                  icon={item.icon}
                  href={item.href}
                  active={isActive(item)}
                  badge={item.badge}
                  onClick={onNavigate}
                />
              ),
            )}
          </SideNavSection>
        ))}
      </nav>

      {/* Footer */}
      <div className="space-y-3 px-3 pb-4">
        <Link
          to="/admin/settings?tab=about"
          onClick={onNavigate}
          className="border-sidebar-border/70 bg-sidebar-accent/40 hover:bg-sidebar-accent/70 block rounded-xl border px-3 py-2 transition-colors"
        >
          <div className="text-muted-foreground text-[10px] font-semibold tracking-[0.18em] uppercase">
            Build
          </div>
          <div className="text-sidebar-foreground mt-1 font-mono text-[12px] leading-5">
            {buildDisplay}
          </div>
          {buildInfo.data?.update_status === "update_available" ? (
            <div className="mt-1 text-[11px] font-medium text-amber-500">Update available</div>
          ) : null}
          {buildInfo.data?.latest_version && buildInfo.data.update_status === "update_available" ? (
            <div className="text-muted-foreground mt-0.5 font-mono text-[11px]">
              Latest {buildInfo.data.latest_version}
            </div>
          ) : null}
        </Link>
        {/* Back to app */}
        <Link
          to="/"
          onClick={onNavigate}
          className="text-muted-foreground hover:text-foreground hover:bg-accent/70 flex items-center gap-2.5 rounded-xl px-3 py-2.5 text-[13px] font-medium transition-colors duration-150"
        >
          <ArrowLeft className="h-[18px] w-[18px]" />
          <span>Back to App</span>
        </Link>
      </div>
    </aside>
  );
}
