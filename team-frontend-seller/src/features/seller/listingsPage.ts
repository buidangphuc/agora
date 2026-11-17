import { ListingStatus } from "@/generated/platform/listing/v1/listing_pb.js";
import { type ListingPage, listMyListings } from "@/lib/gateway/listings";

export const LISTINGS_PAGE_SIZE = 20;

export async function searchMyListings(query: string): Promise<ListingPage> {
  return listMyListings({
    statuses: [ListingStatus.PUBLISHED],
    query,
    pageSize: LISTINGS_PAGE_SIZE,
  });
}
