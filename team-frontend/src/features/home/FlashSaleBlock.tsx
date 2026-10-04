/**
 * Home flash-sale slot. There is no backend source that lists a campaign (the
 * gateway only answers per listing), so no real `endsAt` or sold/stock exists
 * and the block renders nothing. When a campaign-list RPC lands, fetch it here
 * and render `FlashSaleSection` with the real listings, `endsAt` and stock.
 */
export async function FlashSaleBlock() {
  return null;
}
