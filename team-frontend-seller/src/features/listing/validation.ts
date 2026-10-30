/**
 * Listing field rules shared by the form (blur/submit) and saveListingAction,
 * so the inline message and the server message are the same text.
 */
export type SellField = "title" | "price" | "stock" | "categoryId";
export type FieldErrors = Partial<Record<SellField, string>>;

export interface ListingFields {
  title: string;
  price: number;
  stock: number;
  categoryId: string;
}

export const FIELD_MESSAGES: Record<SellField, string> = {
  title: "Tiêu đề bắt buộc.",
  price: "Giá không hợp lệ.",
  stock: "Tồn kho không hợp lệ.",
  categoryId: "Chọn ngành hàng.",
};

/** Errors for the required fields: title, price > 0, stock >= 0, category. */
export function validateListingFields(f: ListingFields): FieldErrors {
  const errors: FieldErrors = {};
  if (!f.title.trim()) errors.title = FIELD_MESSAGES.title;
  if (!Number.isFinite(f.price) || f.price <= 0) {
    errors.price = FIELD_MESSAGES.price;
  }
  if (!Number.isFinite(f.stock) || f.stock < 0) {
    errors.stock = FIELD_MESSAGES.stock;
  }
  if (!f.categoryId.trim()) errors.categoryId = FIELD_MESSAGES.categoryId;
  return errors;
}
