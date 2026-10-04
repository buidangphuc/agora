export type AddressField = "recipientName" | "phone" | "street" | "city";
export type AddressErrors = Partial<Record<AddressField, string>>;

const REQUIRED: [AddressField, string][] = [
  ["recipientName", "Vui lòng nhập họ và tên."],
  ["phone", "Vui lòng nhập số điện thoại."],
  ["street", "Vui lòng nhập địa chỉ chi tiết."],
  ["city", "Vui lòng nhập tỉnh / thành phố."],
];

/** Required-field check for the address form (ward and district are optional). */
export function validateAddress(formData: FormData): AddressErrors {
  const errors: AddressErrors = {};
  for (const [field, message] of REQUIRED) {
    if (String(formData.get(field) ?? "").trim() === "") {
      errors[field] = message;
    }
  }
  return errors;
}

/** First invalid field in form order, for focus management. */
export function firstInvalid(errors: AddressErrors): AddressField | undefined {
  return REQUIRED.map(([field]) => field).find((field) => errors[field]);
}
