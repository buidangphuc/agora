"use client";

import { deleteListingAction } from "./actions";

// A small confirm-then-delete form. The server action is bound to the listing id.
export function DeleteListingButton({ id }: { id: string }) {
  const action = deleteListingAction.bind(null, id);
  return (
    <form
      action={action}
      onSubmit={(e) => {
        if (!confirm("Xoá tin này? Hành động không thể hoàn tác.")) {
          e.preventDefault();
        }
      }}
    >
      <button type="submit" className="text-sm text-red-600 hover:underline">
        Xoá
      </button>
    </form>
  );
}
