import type { FormEvent, ReactNode } from "react";

import type { Library } from "@/api/types";
import { Button } from "@/components/ui/button";

import {
  AdvancedFields,
  FolderFields,
  GeneralFields,
  MetadataFields,
} from "./LibraryFormSections";
import { useLibraryForm } from "./useLibraryForm";

import { Loader2, Save } from "lucide-react";
export interface LibraryFormProps {
  library: Library | null;
  chapterThumbnailsSupported: boolean;
  trickplaySupported: boolean;
  onClose?: () => void;
  onSaved?: (library: Library) => void;
  resetAfterCreate?: boolean;
  submitLabel?: string;
  savingLabel?: string;
}

function FormSection({
  title,
  children,
}: {
  title: string;
  children: ReactNode;
}) {
  return (
    <section className="space-y-3">
      <h3 className="text-muted-foreground text-xs font-semibold tracking-[0.1em] uppercase">
        {title}
      </h3>
      {children}
    </section>
  );
}

/**
 * Inline (non-dialog) library form used by the setup wizard. The admin
 * Libraries page uses LibraryEditorDialog, which renders the same sections
 * behind a left-hand navigation rail.
 */
export function LibraryForm({
  library,
  chapterThumbnailsSupported,
  trickplaySupported,
  onClose,
  onSaved,
  resetAfterCreate = false,
  submitLabel = "Save",
  savingLabel = "Saving...",
}: LibraryFormProps) {
  const form = useLibraryForm({ library, onClose, onSaved, resetAfterCreate });

  function handleSubmit(e: FormEvent) {
    e.preventDefault();
    form.submit();
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-6">
      <FormSection title="General">
        <GeneralFields form={form} />
      </FormSection>
      <FormSection title="Folders">
        <FolderFields form={form} />
      </FormSection>
      <FormSection title="Metadata">
        <MetadataFields form={form} />
      </FormSection>
      <FormSection title="Advanced">
        <AdvancedFields
          form={form}
          chapterThumbnailsSupported={chapterThumbnailsSupported}
          trickplaySupported={trickplaySupported}
        />
      </FormSection>
      <Button type="submit" className="w-full" disabled={form.isPending}>
        {form.isPending ? <Loader2 className="animate-spin" /> : <Save />}
        {form.isPending ? savingLabel : submitLabel}
      </Button>
    </form>
  );
}
