import { createContext, useContext, useEffect, useRef, useState } from "react";
import {
  DndContext,
  DragOverlay,
  closestCenter,
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
} from "@dnd-kit/core";
import type { DragEndEvent, DragStartEvent } from "@dnd-kit/core";
import {
  SortableContext,
  arrayMove,
  verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import type { LibraryCollection, LibraryCollectionGroup } from "@/api/types";
import {
  useReorderCollectionGroups,
  useReorderCollectionsInGroup,
} from "@/hooks/queries/admin/collectionGroups";
import { GroupCard } from "./GroupCard";
import { UngroupedSection } from "./UngroupedSection";

// ---------------------------------------------------------------------------
// Selection context
// ---------------------------------------------------------------------------

export type SelectionKind = "collection" | "user_collection";

interface AnchorRef {
  id: string;
  groupID: string;
}

export interface SelectionContextValue {
  selectedIds: Set<string>;
  selectionKind: SelectionKind | null;
  isSelected: (id: string) => boolean;
  selectOnly: (id: string, kind: SelectionKind, groupID: string) => void;
  toggleOne: (id: string, kind: SelectionKind, groupID: string) => void;
  selectRange: (
    id: string,
    kind: SelectionKind,
    groupID: string,
    groupCollectionIDs: string[],
  ) => void;
  clear: () => void;
}

export const SelectionContext = createContext<SelectionContextValue | null>(
  null,
);

export function useSelection(): SelectionContextValue {
  const ctx = useContext(SelectionContext);
  if (!ctx)
    throw new Error(
      "useSelection must be used inside SelectionContext.Provider",
    );
  return ctx;
}

// ---------------------------------------------------------------------------
// Board types
// ---------------------------------------------------------------------------

interface BoardGroup extends LibraryCollectionGroup {
  collections: LibraryCollection[];
}

type UnifiedItem = { kind: "group"; group: BoardGroup } | { kind: "ungrouped" };

export interface GroupsBoardProps {
  libraryID: number;
  groups: BoardGroup[];
  ungrouped: LibraryCollection[];
  ungroupedSortOrder: number;
  onEditGroup: (id: string) => void;
  onEditCollection: (collection: LibraryCollection) => void;
  onDeleteCollection: (collection: LibraryCollection) => void;
  onSyncCollection: (collection: LibraryCollection) => void;
  syncingCollectionID?: string | null;
}

// ---------------------------------------------------------------------------
// GroupsBoard
// ---------------------------------------------------------------------------

export function GroupsBoard({
  libraryID,
  groups,
  ungrouped,
  ungroupedSortOrder,
  onEditGroup,
  onEditCollection,
  onDeleteCollection,
  onSyncCollection,
  syncingCollectionID = null,
}: GroupsBoardProps) {
  // --- selection state ---
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [selectionKind, setSelectionKind] = useState<SelectionKind | null>(
    null,
  );
  const anchorRef = useRef<AnchorRef | null>(null);

  const isSelected = (id: string) => selectedIds.has(id);

  const selectOnly = (id: string, kind: SelectionKind, groupID: string) => {
    setSelectedIds(new Set([id]));
    setSelectionKind(kind);
    anchorRef.current = { id, groupID };
  };

  const toggleOne = (id: string, kind: SelectionKind, groupID: string) => {
    if (selectionKind !== null && kind !== selectionKind) {
      // cross-kind: replace with just this item
      setSelectedIds(new Set([id]));
      setSelectionKind(kind);
      anchorRef.current = { id, groupID };
      return;
    }
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
    setSelectionKind(kind);
    anchorRef.current = { id, groupID };
  };

  const selectRange = (
    id: string,
    kind: SelectionKind,
    groupID: string,
    groupCollectionIDs: string[],
  ) => {
    const anchor = anchorRef.current;
    if (
      !anchor ||
      anchor.groupID !== groupID ||
      (selectionKind !== null && kind !== selectionKind)
    ) {
      // No valid anchor in this group — fall back to selectOnly
      selectOnly(id, kind, groupID);
      return;
    }
    const anchorIdx = groupCollectionIDs.indexOf(anchor.id);
    const currentIdx = groupCollectionIDs.indexOf(id);
    if (anchorIdx === -1 || currentIdx === -1) {
      selectOnly(id, kind, groupID);
      return;
    }
    const lo = Math.min(anchorIdx, currentIdx);
    const hi = Math.max(anchorIdx, currentIdx);
    const rangeIds = groupCollectionIDs.slice(lo, hi + 1);
    setSelectedIds((prev) => {
      const next = new Set(prev);
      for (const rid of rangeIds) next.add(rid);
      return next;
    });
    setSelectionKind(kind);
    // anchor stays unchanged on range extends
  };

  const clear = () => {
    setSelectedIds(new Set());
    setSelectionKind(null);
    anchorRef.current = null;
  };

  const selection: SelectionContextValue = {
    selectedIds,
    selectionKind,
    isSelected,
    selectOnly,
    toggleOne,
    selectRange,
    clear,
  };

  // --- Escape key to clear selection ---
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") clear();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  // --- dnd state ---
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(KeyboardSensor),
  );
  const reorderGroups = useReorderCollectionGroups(libraryID);
  const reorderCollections = useReorderCollectionsInGroup(libraryID);
  const [activeId, setActiveId] = useState<string | null>(null);
  const [draggedIds, setDraggedIds] = useState<string[]>([]);

  // Build a unified ordering of groups + the ungrouped sentinel, sorted by
  // each item's effective sort_order position.
  const unifiedItems: UnifiedItem[] = buildUnifiedItems(
    groups,
    ungroupedSortOrder,
  );
  const sortableIds = unifiedItems.map((item) =>
    item.kind === "ungrouped" ? "ungrouped" : `group:${item.group.id}`,
  );

  const onDragStart = (e: DragStartEvent) => {
    const id = String(e.active.id);
    setActiveId(id);

    const aData = e.active.data.current as
      { kind?: string; id?: string } | undefined;
    if (aData?.kind === "collection" && aData.id) {
      if (selectedIds.has(aData.id)) {
        // Drag the whole selection in visual order
        setDraggedIds(
          flattenedSelectionInVisualOrder(groups, ungrouped, selectedIds),
        );
      } else {
        // Dragging an unselected row — clear selection, drag just this one
        clear();
        setDraggedIds([aData.id]);
      }
    } else {
      setDraggedIds([]);
    }
  };

  const onDragCancel = () => {
    setActiveId(null);
    setDraggedIds([]);
  };

  // While a section (group or ungrouped) is being dragged, collapse all
  // sections to just their headers — long groups otherwise force the user to
  // scroll past dozens of rows to reorder. Collection drags don't trigger
  // collapse (admin needs to see destination bodies as drop targets).
  const isSectionDrag =
    activeId !== null &&
    (activeId === "ungrouped" || activeId.startsWith("group:"));

  const onDragEnd = (e: DragEndEvent) => {
    setActiveId(null);
    const { active, over } = e;
    if (!over || active.id === over.id) {
      setDraggedIds([]);
      return;
    }

    const aData = active.data.current as
      { kind?: string; id?: string } | undefined;
    const oData = over.data.current as
      { kind?: string; id?: string; groupID?: string } | undefined;
    if (!aData || !oData) {
      setDraggedIds([]);
      return;
    }

    // Resolve which group-section the drop target belongs to, regardless of
    // whether collision detection picked the outer sortable ("group") or the
    // inner droppable body ("group-body"). Returns the group's ID (or
    // "ungrouped" sentinel), or null if the over isn't a section at all.
    const overSectionId = (() => {
      if (oData.kind === "group") return oData.id ?? null;
      if (oData.kind === "group-body") return oData.groupID ?? null;
      return null;
    })();

    // Group / ungrouped section reorder
    if (aData.kind === "group" && overSectionId !== null) {
      // Build the current ordered ID list (including "ungrouped" sentinel)
      const currentIds = sortableIds.map((sid) =>
        sid === "ungrouped" ? "ungrouped" : sid.replace(/^group:/, ""),
      );
      const activeItemId =
        active.id === "ungrouped"
          ? "ungrouped"
          : String(active.id).replace(/^group:/, "");

      const oldIdx = currentIds.indexOf(activeItemId);
      const newIdx = currentIds.indexOf(overSectionId);
      if (oldIdx !== -1 && newIdx !== -1 && oldIdx !== newIdx) {
        reorderGroups.mutate(arrayMove(currentIds, oldIdx, newIdx));
      }
      setDraggedIds([]);
      return;
    }

    // Collection drag
    if (aData.kind === "collection") {
      const sourceCollId = aData.id ?? "";
      let targetGroupId: string | null = null;
      if (oData.kind === "collection") {
        targetGroupId = findGroupOfCollection(
          groups,
          ungrouped,
          oData.id ?? "",
        );
      } else if (overSectionId !== null) {
        // Group-body drops AND drops on a group's outer sortable both resolve
        // to the section's ID — admin who drops on a group header still gets
        // the move applied to that group.
        targetGroupId = overSectionId;
      }
      if (!targetGroupId) {
        setDraggedIds([]);
        return;
      }

      const sourceGroupId = findGroupOfCollection(
        groups,
        ungrouped,
        sourceCollId,
      );
      if (isCrossKind(groups, sourceGroupId, targetGroupId)) {
        setDraggedIds([]);
        return;
      }

      const effectiveDraggedIds =
        draggedIds.length > 0 ? draggedIds : [sourceCollId];
      const newOrder = computeNewOrder(
        groups,
        ungrouped,
        targetGroupId,
        effectiveDraggedIds,
        oData,
      );
      reorderCollections.mutate({
        groupID: targetGroupId,
        orderedIDs: newOrder,
        ...(targetGroupId === "ungrouped" ? { libraryId: libraryID } : {}),
      });

      // Clear selection after successful drop
      clear();
    }

    setDraggedIds([]);
  };

  return (
    <SelectionContext.Provider value={selection}>
      <DndContext
        sensors={sensors}
        collisionDetection={closestCenter}
        onDragStart={onDragStart}
        onDragCancel={onDragCancel}
        onDragEnd={onDragEnd}
      >
        <SortableContext
          items={sortableIds}
          strategy={verticalListSortingStrategy}
        >
          <div className="space-y-4">
            {unifiedItems.map((item) =>
              item.kind === "ungrouped" ? (
                <UngroupedSection
                  key="ungrouped"
                  collections={ungrouped}
                  collapsed={isSectionDrag}
                  onEditCollection={onEditCollection}
                  onDeleteCollection={onDeleteCollection}
                  onSyncCollection={onSyncCollection}
                  syncingCollectionID={syncingCollectionID}
                />
              ) : (
                <GroupCard
                  key={item.group.id}
                  group={item.group}
                  collections={item.group.collections}
                  onEdit={onEditGroup}
                  onEditCollection={onEditCollection}
                  onDeleteCollection={onDeleteCollection}
                  onSyncCollection={onSyncCollection}
                  syncingCollectionID={syncingCollectionID}
                  collapsed={isSectionDrag}
                />
              ),
            )}
          </div>
        </SortableContext>
        <DragOverlay>
          {activeId && draggedIds.length > 1 ? (
            <div className="bg-card rounded-md border px-3 py-2 text-sm font-medium shadow-lg">
              {draggedIds.length} collections selected
            </div>
          ) : null}
        </DragOverlay>
        {activeId && <div className="sr-only">Dragging {activeId}</div>}
      </DndContext>
    </SelectionContext.Provider>
  );
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/**
 * Build a unified array of groups + the ungrouped sentinel, ordered by each
 * item's effective sort position. Groups use their sort_order; ungrouped uses
 * ungroupedSortOrder. Ties are broken by name for groups (ungrouped always
 * sorts last within ties since it has no name).
 */
function buildUnifiedItems(
  groups: BoardGroup[],
  ungroupedSortOrder: number,
): UnifiedItem[] {
  type Slot = { order: number; item: UnifiedItem };
  const slots: Slot[] = groups.map((g) => ({
    order: g.sort_order,
    item: { kind: "group" as const, group: g },
  }));
  slots.push({
    order: ungroupedSortOrder,
    item: { kind: "ungrouped" as const },
  });
  slots.sort((a, b) => {
    if (a.order !== b.order) return a.order - b.order;
    // Equal sort_order: put named groups before ungrouped; stable between groups by name
    const aName = a.item.kind === "group" ? a.item.group.name : "￿";
    const bName = b.item.kind === "group" ? b.item.group.name : "￿";
    return aName.localeCompare(bName);
  });
  return slots.map((s) => s.item);
}

function findGroupOfCollection(
  groups: BoardGroup[],
  ungrouped: LibraryCollection[],
  collectionId: string,
): string | null {
  for (const g of groups)
    if (g.collections.some((c) => c.id === collectionId)) return g.id;
  if (ungrouped.some((c) => c.id === collectionId)) return "ungrouped";
  return null;
}

function isCrossKind(
  groups: BoardGroup[],
  sourceGroupId: string | null,
  targetGroupId: string,
): boolean {
  if (
    !sourceGroupId ||
    sourceGroupId === "ungrouped" ||
    targetGroupId === "ungrouped"
  )
    return false;
  const src = groups.find((g) => g.id === sourceGroupId);
  const tgt = groups.find((g) => g.id === targetGroupId);
  return !!src && !!tgt && src.kind !== tgt.kind;
}

function computeNewOrder(
  groups: BoardGroup[],
  ungrouped: LibraryCollection[],
  targetGroupId: string,
  movedIds: string[],
  over: { kind?: string; id?: string },
): string[] {
  const targetList =
    targetGroupId === "ungrouped"
      ? ungrouped.map((c) => c.id)
      : (groups
          .find((g) => g.id === targetGroupId)
          ?.collections.map((c) => c.id) ?? []);
  const movedSet = new Set(movedIds);
  const filtered = targetList.filter((id) => !movedSet.has(id));
  if (over.kind === "collection" && over.id) {
    const idx = filtered.indexOf(over.id);
    if (idx === -1) return [...filtered, ...movedIds];
    return [...filtered.slice(0, idx), ...movedIds, ...filtered.slice(idx)];
  }
  return [...filtered, ...movedIds];
}

/**
 * Walk groups in display order, then ungrouped, collecting IDs that are in
 * selectedIds. This gives a stable, deterministic drag order.
 */
function flattenedSelectionInVisualOrder(
  groups: BoardGroup[],
  ungrouped: LibraryCollection[],
  selectedIds: Set<string>,
): string[] {
  const result: string[] = [];
  for (const g of groups) {
    for (const c of g.collections) {
      if (selectedIds.has(c.id)) result.push(c.id);
    }
  }
  for (const c of ungrouped) {
    if (selectedIds.has(c.id)) result.push(c.id);
  }
  return result;
}
