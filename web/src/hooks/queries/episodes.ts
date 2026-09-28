import {
  useCatalogItemEpisodes,
  useCatalogSeasonDetail,
  useCatalogSeasonEpisodes,
  useCatalogSeriesSeasons,
} from "./catalogRead";

export function useSeasons(seriesId: string | undefined) {
  return useCatalogSeriesSeasons(seriesId);
}

export function useSeasonEpisodes(seriesId: string | undefined, seasonNum: number) {
  return useCatalogSeasonEpisodes(seriesId, seasonNum);
}

export function useItemEpisodes(itemId: string | undefined) {
  return useCatalogItemEpisodes(itemId);
}

export function useSeasonDetail(seriesId: string | undefined, seasonNum: number) {
  return useCatalogSeasonDetail(seriesId, seasonNum);
}
