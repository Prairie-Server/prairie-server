import { Link, Navigate } from "react-router";
import { useUserLibraries } from "@/hooks/queries/libraries";

export default function HomeRedirect() {
  const { data: libraries, isLoading } = useUserLibraries();

  if (isLoading) {
    return (
      <div className="flex h-full items-center justify-center">
        <div className="border-primary h-8 w-8 animate-spin rounded-full border-b-2" />
      </div>
    );
  }

  const firstLibrary = libraries?.[0];
  if (firstLibrary) {
    return <Navigate to={`/library/${firstLibrary.id}`} replace />;
  }

  return (
    <div className="text-muted-foreground flex h-full flex-col items-center justify-center gap-2 px-6 text-center">
      <p>No visible libraries are available right now.</p>
      <Link to="/settings/libraries" className="text-primary text-sm font-medium hover:underline">
        Manage library visibility in Settings
      </Link>
    </div>
  );
}
