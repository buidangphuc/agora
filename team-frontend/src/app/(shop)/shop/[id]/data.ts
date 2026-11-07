import { getStorefront } from "@/lib/gateway/listings";
import { requestCache } from "@/lib/requestCache";

export const loadStorefront = requestCache(getStorefront);
