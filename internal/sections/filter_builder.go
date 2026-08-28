package sections

import "github.com/prairie-server/prairie-server/internal/catalog"

type FilterBuilder = catalog.QueryBuilder

func NewFilterBuilder(alias string) *catalog.QueryBuilder {
	return catalog.NewQueryBuilder(alias)
}
