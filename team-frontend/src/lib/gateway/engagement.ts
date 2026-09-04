/**
 * Server-side engagement calls (favorites + views/stats + wishlist collections)
 * through the gateway (ARCHITECTURE Rule 1). No business logic here — this module
 * only forwards to team-engagement and maps proto → plain view types.
 */
import "server-only";

import type {
  Collection,
  ProductAnswer,
  ProductQuestion,
} from "@/generated/platform/engagement/v1/engagement_pb.js";
import { makeClients } from "./client.js";
import { getToken } from "./session.js";

function gateway() {
  return makeClients(getToken());
}

export interface ListingStats {
  viewCount: number;
  favoriteCount: number;
}

export interface ViewCollection {
  id: string;
  name: string;
  itemCount: number;
  createdAt: string;
}

function mapCollection(c: Collection): ViewCollection {
  let createdAt = "";
  if (c.createdAt) {
    createdAt = new Date(Number(c.createdAt.seconds) * 1000).toLocaleDateString(
      "vi-VN",
    );
  }
  return {
    id: c.id,
    name: c.name,
    itemCount: Number(c.itemCount),
    createdAt,
  };
}

export async function addFavorite(listingId: string): Promise<void> {
  await gateway().engagement.addFavorite({ listingId });
}

export async function removeFavorite(listingId: string): Promise<void> {
  await gateway().engagement.removeFavorite({ listingId });
}

export async function isFavorite(listingId: string): Promise<boolean> {
  try {
    const res = await gateway().engagement.isFavorite({ listingId });
    return res.favorite;
  } catch {
    return false; // anonymous / not logged in
  }
}

export async function listFavoriteIds(
  cursor = "",
): Promise<{ ids: string[]; nextCursor: string; total: number }> {
  const res = await gateway().engagement.listFavorites({
    page: { cursor, pageSize: 24 },
  });
  return {
    ids: res.listingIds,
    nextCursor: res.page?.nextCursor ?? "",
    total: Number(res.page?.total ?? 0n),
  };
}

export async function recordView(listingId: string): Promise<void> {
  try {
    await gateway().engagement.recordView({ listingId });
  } catch {
    // best-effort; a failed view record shouldn't break the page
  }
}

/** Recently-viewed listing ids for the current user (most-recent first). */
export async function getRecentlyViewed(limit = 12): Promise<string[]> {
  try {
    const res = await gateway().engagement.getRecentlyViewed({
      page: { cursor: "", pageSize: limit },
    });
    return res.listingIds;
  } catch {
    return []; // anonymous / not logged in / service unavailable
  }
}

export async function getStats(listingId: string): Promise<ListingStats> {
  try {
    const res = await gateway().engagement.getListingStats({ listingId });
    return {
      viewCount: Number(res.viewCount),
      favoriteCount: Number(res.favoriteCount),
    };
  } catch {
    return { viewCount: 0, favoriteCount: 0 };
  }
}

// ── Wishlist collections (named lists on top of favorites) ──────────────────

export async function createCollection(name: string): Promise<ViewCollection> {
  const res = await gateway().engagement.createCollection({
    name: name.trim(),
  });
  if (!res.collection) throw new Error("create collection failed");
  return mapCollection(res.collection);
}

export async function listCollections(): Promise<ViewCollection[]> {
  try {
    const res = await gateway().engagement.listCollections({});
    return res.collections.map(mapCollection);
  } catch {
    return []; // anonymous / not logged in
  }
}

export async function addToCollection(
  collectionId: string,
  listingId: string,
): Promise<void> {
  await gateway().engagement.addToCollection({ collectionId, listingId });
}

export async function removeFromCollection(
  collectionId: string,
  listingId: string,
): Promise<void> {
  await gateway().engagement.removeFromCollection({ collectionId, listingId });
}

export async function listCollectionItems(
  collectionId: string,
  cursor = "",
): Promise<{ ids: string[]; nextCursor: string; total: number }> {
  try {
    const res = await gateway().engagement.listCollectionItems({
      collectionId,
      page: { cursor, pageSize: 24 },
    });
    return {
      ids: res.listingIds,
      nextCursor: res.page?.nextCursor ?? "",
      total: Number(res.page?.total ?? 0n),
    };
  } catch {
    return { ids: [], nextCursor: "", total: 0 };
  }
}

// ── Listing Q&A (buyer questions + seller/shop answers) ─────────────────────

export interface ViewAnswer {
  id: string;
  questionId: string;
  userId: string;
  answerText: string;
  isShopReply: boolean;
  createdAt: string;
}

export interface ViewQuestion {
  id: string;
  listingId: string;
  userId: string;
  questionText: string;
  answers: ViewAnswer[];
  createdAt: string;
}

function tsToDate(ts?: { seconds: bigint }): string {
  if (!ts) return "";
  return new Date(Number(ts.seconds) * 1000).toLocaleDateString("vi-VN");
}

function mapAnswer(a: ProductAnswer): ViewAnswer {
  return {
    id: a.id,
    questionId: a.questionId,
    userId: a.userId,
    answerText: a.answerText,
    isShopReply: a.isShopReply,
    createdAt: tsToDate(a.createdAt),
  };
}

function mapQuestion(q: ProductQuestion): ViewQuestion {
  return {
    id: q.id,
    listingId: q.listingId,
    userId: q.userId,
    questionText: q.questionText,
    answers: q.answers.map(mapAnswer),
    createdAt: tsToDate(q.createdAt),
  };
}

export async function askQuestion(
  listingId: string,
  questionText: string,
): Promise<ViewQuestion> {
  const res = await gateway().engagement.askQuestion({
    listingId,
    questionText,
  });
  if (!res.question) throw new Error("ask question failed");
  return mapQuestion(res.question);
}

export async function answerQuestion(
  questionId: string,
  answerText: string,
  isShopReply = false,
): Promise<ViewAnswer> {
  const res = await gateway().engagement.answerQuestion({
    questionId,
    answerText,
    isShopReply,
  });
  if (!res.answer) throw new Error("answer question failed");
  return mapAnswer(res.answer);
}

export async function listQuestionsByListing(
  listingId: string,
): Promise<ViewQuestion[]> {
  try {
    const res = await gateway().engagement.listQuestionsByListing({
      listingId,
      page: { cursor: "", pageSize: 24 },
    });
    return res.questions.map(mapQuestion);
  } catch {
    return [];
  }
}

// ── Follow sellers (buyer follows shop) ─────────────────────────────────────

export async function followSeller(sellerId: string): Promise<void> {
  await gateway().engagement.followSeller({ sellerId });
}

export async function unfollowSeller(sellerId: string): Promise<void> {
  await gateway().engagement.unfollowSeller({ sellerId });
}

export async function isFollowing(sellerId: string): Promise<boolean> {
  try {
    const res = await gateway().engagement.isFollowing({ sellerId });
    return res.following;
  } catch {
    return false; // anonymous / not logged in
  }
}

export async function listFollowedSellers(): Promise<string[]> {
  try {
    const res = await gateway().engagement.listFollowedSellers({
      page: { cursor: "", pageSize: 48 },
    });
    return res.sellerIds;
  } catch {
    return [];
  }
}

export async function listFollowedListings(): Promise<string[]> {
  try {
    const res = await gateway().engagement.listFollowedListings({
      page: { cursor: "", pageSize: 48 },
    });
    return res.listingIds;
  } catch {
    return [];
  }
}

// ── Loyalty / daily check-in ────────────────────────────────────────────────
// team-engagement owns the streak/coin ledger; this module just forwards.

export interface ViewLoyalty {
  streak: number;
  coinBalance: number;
  lastCheckin: string;
}

export interface CheckInResult {
  streak: number;
  coinsEarned: number;
  coinBalance: number;
}

export async function getLoyalty(): Promise<ViewLoyalty> {
  try {
    const res = await gateway().engagement.getLoyalty({});
    let lastCheckin = "";
    if (res.lastCheckin) {
      lastCheckin = new Date(
        Number(res.lastCheckin.seconds) * 1000,
      ).toLocaleDateString("vi-VN");
    }
    return {
      streak: res.streak,
      coinBalance: Number(res.coinBalance),
      lastCheckin,
    };
  } catch {
    return { streak: 0, coinBalance: 0, lastCheckin: "" };
  }
}

export async function checkIn(): Promise<CheckInResult> {
  const res = await gateway().engagement.checkIn({});
  return {
    streak: res.streak,
    coinsEarned: Number(res.coinsEarned),
    coinBalance: Number(res.coinBalance),
  };
}
