import type { SmartCollectionAccess } from "@/api/types";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";

interface CollectionAccessEditorProps {
  value: SmartCollectionAccess;
  onChange: (value: SmartCollectionAccess) => void;
  profiles?: Array<{ id: string; name: string }>;
  readOnly?: boolean;
  creatorProfileId?: string | null;
}

export default function CollectionAccessEditor({
  value,
  onChange,
  profiles = [],
  readOnly = false,
  creatorProfileId,
}: CollectionAccessEditorProps) {
  return (
    <div className="space-y-4">
      {readOnly ? (
        <div className="text-muted-foreground rounded-md border px-3 py-2 text-sm">
          Only the creator can edit this collection.
          {creatorProfileId ? ` Created by ${creatorProfileId}.` : ""}
        </div>
      ) : null}

      <div className="border-border flex items-center justify-between rounded-lg border px-4 py-3">
        <div>
          <Label className="text-sm font-medium">Share with this account</Label>
          <p className="text-muted-foreground mt-1 text-xs">
            Shared collections appear for the selected profiles across the account.
          </p>
        </div>
        <Switch
          checked={value.is_shared}
          onCheckedChange={(checked) =>
            onChange({
              ...value,
              is_shared: checked,
              allowed_profile_ids: checked ? value.allowed_profile_ids : [],
            })
          }
          disabled={readOnly}
        />
      </div>

      {value.is_shared && profiles.length > 0 ? (
        <div className="space-y-2">
          <Label>Allowed Profiles</Label>
          <div className="flex flex-wrap gap-2">
            {profiles.map((profile) => {
              const selected = value.allowed_profile_ids.includes(profile.id);
              return (
                <Button
                  key={profile.id}
                  type="button"
                  variant={selected ? "default" : "outline"}
                  size="sm"
                  disabled={readOnly}
                  onClick={() =>
                    onChange({
                      ...value,
                      allowed_profile_ids: selected
                        ? value.allowed_profile_ids.filter((id) => id !== profile.id)
                        : [...value.allowed_profile_ids, profile.id],
                    })
                  }
                >
                  {profile.name}
                </Button>
              );
            })}
          </div>
        </div>
      ) : null}
    </div>
  );
}
