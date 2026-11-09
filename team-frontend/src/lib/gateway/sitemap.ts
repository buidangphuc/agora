import "server-only";

import { SortBy } from "@/generated/platform/search/v1/search_pb.js";

import { makeClients } from "./client.js";
import { getToken } from "./session.js";

export const SITEMAP_PAGE_SIZE = 50;
export const SITEMAP_MAX_LISTINGS = 5000;

export interface SitemapListing {
  listingId: string;
  sellerId: string;
}

export async function readSitemapListings(): Promise<SitemapListing[]> {
  const search = makeClients(getToken(), { anonymousFallback: true }).search;
  const out: SitemapListing[] = [];
  let cursor = "";
  try {
    while (out.length < SITEMAP_MAX_LISTINGS) {
      const res = await search.searchListings({
        query: "",
        filters: { status: "published" },
        sortBy: SortBy.NEWEST,
        page: { cursor, pageSize: SITEMAP_PAGE_SIZE },
      });
      for (const h of res.hits) {
        if (out.length >= SITEMAP_MAX_LISTINGS) break;
        out.push({ listingId: h.listingId, sellerId: h.sellerId });
      }
      cursor = res.page?.nextCursor ?? "";
      if (!cursor || res.hits.length === 0) break;
    }
  } catch (err) {
    console.warn("sitemap listing read failed", err);
  }
  return out;
}
