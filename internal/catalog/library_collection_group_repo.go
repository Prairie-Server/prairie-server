package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/prairie-server/prairie-server/internal/collectionutil"
	"github.com/prairie-server/prairie-server/internal/idgen"
	"github.com/prairie-server/prairie-server/internal/models"
)

var ErrLibraryCollectionGroupNotFound = errors.New("library collection group not found")

type LibraryCollectionGroupRepository struct {
	pool *pgxpool.Pool
}

func NewLibraryCollectionGroupRepository(pool *pgxpool.Pool) *LibraryCollectionGroupRepository {
	return &LibraryCollectionGroupRepository{pool: pool}
}

type CreateLibraryCollectionGroupInput struct {
	LibraryID       int
	Name            string
	Slug            string
	Kind            models.LibraryCollectionGroupKind
	DefaultSortMode models.GroupSortMode
}

type UpdateLibraryCollectionGroupInput struct {
	Name            *string
	Slug            *string
	DefaultSortMode *models.GroupSortMode
}

const libraryCollectionGroupColumns = `id, library_id, name, slug, kind, default_sort_mode, sort_order, created_at, updated_at`

type groupRowScanner interface {
	Scan(dest ...any) error
}

func scanLibraryCollectionGroup(row groupRowScanner) (*models.LibraryCollectionGroup, error) {
	var g models.LibraryCollectionGroup
	if err := row.Scan(
		&g.ID,
		&g.LibraryID,
		&g.Name,
		&g.Slug,
		&g.Kind,
		&g.DefaultSortMode,
		&g.SortOrder,
		&g.CreatedAt,
		&g.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &g, nil
}

func (r *LibraryCollectionGroupRepository) Create(ctx context.Context, in CreateLibraryCollectionGroupInput) (*models.LibraryCollectionGroup, error) {
	if in.Name == "" {
		return nil, fmt.Errorf("group name is required")
	}
	if in.Slug == "" {
		in.Slug = collectionutil.SlugifyGroupSlug(in.Name)
	}
	if in.Kind == "" {
		in.Kind = models.GroupKindRegular
	}
	if in.DefaultSortMode == "" {
		in.DefaultSortMode = models.GroupSortManual
	}
	id, err := idgen.NextID()
	if err != nil {
		return nil, fmt.Errorf("idgen: %w", err)
	}

	row := r.pool.QueryRow(ctx, `
		INSERT INTO library_collection_groups (
			id, library_id, label, title, name, slug, kind, default_sort_mode, sort_order
		)
		VALUES ($1, $2, $3, $4, $4, $3, $5, $6,
			COALESCE((SELECT MAX(sort_order) + 1 FROM library_collection_groups WHERE library_id = $2), 0))
		RETURNING `+libraryCollectionGroupColumns,
		id, in.LibraryID, in.Slug, in.Name, in.Kind, in.DefaultSortMode)
	return scanLibraryCollectionGroup(row)
}

func (r *LibraryCollectionGroupRepository) GetByID(ctx context.Context, id string) (*models.LibraryCollectionGroup, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+libraryCollectionGroupColumns+` FROM library_collection_groups WHERE id = $1`, id)
	g, err := scanLibraryCollectionGroup(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrLibraryCollectionGroupNotFound
	}
	return g, err
}

func (r *LibraryCollectionGroupRepository) ListByLibrary(ctx context.Context, libraryID int) ([]models.LibraryCollectionGroup, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+libraryCollectionGroupColumns+`
		FROM library_collection_groups
		WHERE library_id = $1
		ORDER BY sort_order ASC, name ASC, id ASC`, libraryID)
	if err != nil {
		return nil, fmt.Errorf("listing library collection groups: %w", err)
	}
	defer rows.Close()

	out := []models.LibraryCollectionGroup{}
	for rows.Next() {
		g, err := scanLibraryCollectionGroup(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning library collection group: %w", err)
		}
		out = append(out, *g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating library collection groups: %w", err)
	}
	return out, nil
}

func (r *LibraryCollectionGroupRepository) GetUngroupedSortOrder(ctx context.Context, libraryID int) (int, error) {
	var sortOrder int
	if err := r.pool.QueryRow(ctx, `
		SELECT collection_ungrouped_sort_order
		FROM media_folders
		WHERE id = $1
	`, libraryID).Scan(&sortOrder); err != nil {
		return 0, fmt.Errorf("loading ungrouped collection sort order: %w", err)
	}
	return sortOrder, nil
}

func (r *LibraryCollectionGroupRepository) Update(ctx context.Context, id string, in UpdateLibraryCollectionGroupInput) (*models.LibraryCollectionGroup, error) {
	sets := []string{}
	args := []any{id}
	pos := 2
	if in.Name != nil {
		sets = append(sets, fmt.Sprintf("name = $%d", pos), fmt.Sprintf("title = $%d", pos))
		args = append(args, *in.Name)
		pos++
	}
	if in.Slug != nil {
		sets = append(sets, fmt.Sprintf("slug = $%d", pos), fmt.Sprintf("label = $%d", pos))
		args = append(args, *in.Slug)
		pos++
	}
	if in.DefaultSortMode != nil {
		sets = append(sets, fmt.Sprintf("default_sort_mode = $%d", pos))
		args = append(args, *in.DefaultSortMode)
		pos++
	}
	if len(sets) == 0 {
		return r.GetByID(ctx, id)
	}
	sets = append(sets, fmt.Sprintf("updated_at = $%d", pos))
	args = append(args, time.Now())

	q := fmt.Sprintf(`UPDATE library_collection_groups SET %s WHERE id = $1 RETURNING `+libraryCollectionGroupColumns, strings.Join(sets, ", "))
	row := r.pool.QueryRow(ctx, q, args...)
	g, err := scanLibraryCollectionGroup(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrLibraryCollectionGroupNotFound
	}
	return g, err
}

func (r *LibraryCollectionGroupRepository) Delete(ctx context.Context, id string) error {
	g, err := r.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if g.Kind == models.GroupKindUserCollections {
		return fmt.Errorf("user-collections group cannot be deleted")
	}
	tag, err := r.pool.Exec(ctx, `DELETE FROM library_collection_groups WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting library collection group: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrLibraryCollectionGroupNotFound
	}
	return nil
}

func (r *LibraryCollectionGroupRepository) Reorder(ctx context.Context, libraryID int, orderedIDs []string) error {
	if len(orderedIDs) == 0 {
		return collectionutil.ErrOrderedIDsMismatch
	}
	if collectionutil.HasDuplicateOrderedIDs(orderedIDs) {
		return fmt.Errorf("ordered_ids contains duplicates")
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning group reorder: %w", err)
	}
	defer tx.Rollback(ctx)

	realIDs := make([]string, 0, len(orderedIDs))
	realPositions := make([]int, 0, len(orderedIDs))
	ungroupedIdx := -1
	for idx, id := range orderedIDs {
		if id == "ungrouped" {
			if ungroupedIdx >= 0 {
				return fmt.Errorf("ordered_ids contains duplicate ungrouped sentinel")
			}
			ungroupedIdx = idx
			continue
		}
		realIDs = append(realIDs, id)
		realPositions = append(realPositions, idx)
	}

	var total int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM library_collection_groups WHERE library_id = $1`, libraryID).Scan(&total); err != nil {
		return fmt.Errorf("counting library collection groups: %w", err)
	}
	if total != len(realIDs) {
		return collectionutil.ErrOrderedIDsMismatch
	}

	if len(realIDs) > 0 {
		var found int
		if err := tx.QueryRow(ctx, `
			WITH supplied AS (
			  SELECT *
			  FROM unnest($1::text[], $2::integer[]) AS u(id, pos)
			),
			upd AS (
			  UPDATE library_collection_groups g
			  SET sort_order = supplied.pos, updated_at = NOW()
			  FROM supplied
			  WHERE g.library_id = $3 AND g.id = supplied.id
			  RETURNING 1
			)
			SELECT COUNT(*) FROM upd
		`, realIDs, realPositions, libraryID).Scan(&found); err != nil {
			return fmt.Errorf("reordering library collection groups: %w", err)
		}
		if found != len(realIDs) {
			return collectionutil.ErrOrderedIDsMismatch
		}
	}

	if ungroupedIdx >= 0 {
		if _, err := tx.Exec(ctx, `
			UPDATE media_folders
			SET collection_ungrouped_sort_order = $1
			WHERE id = $2
		`, ungroupedIdx, libraryID); err != nil {
			return fmt.Errorf("updating ungrouped collection position: %w", err)
		}
	}

	return tx.Commit(ctx)
}
