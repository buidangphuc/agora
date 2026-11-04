"use server";

export async function acceptSellerTerms(
  _termsVersion: string,
): Promise<{ error: string }> {
  return { error: "" };
}
