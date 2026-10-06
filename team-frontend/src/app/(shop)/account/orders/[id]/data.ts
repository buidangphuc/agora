import { getOrderResult } from "@/lib/gateway/orders";
import { requestCache } from "@/lib/requestCache";

/** One gateway read per request, shared by the segment layout and the page. */
export const loadOrderResult = requestCache(getOrderResult);
