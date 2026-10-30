import { getListing } from "@/lib/gateway/listings";
import { requestCache } from "@/lib/requestCache";

/** One gateway read per request, shared by the segment layout and the page. */
export const loadListing = requestCache(getListing);
