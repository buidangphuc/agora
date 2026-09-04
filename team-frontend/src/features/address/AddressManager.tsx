"use client";

import { useState } from "react";

import type { ViewAddress } from "@/lib/gateway/addresses";
import { AddressModal } from "./AddressModal";
import { deleteAddressAction, setDefaultAddressAction } from "./actions";

export function AddressManager({
  addresses,
}: {
  addresses: ViewAddress[];
}) {
  const [modalOpen, setModalOpen] = useState(false);
  const [editingAddress, setEditingAddress] = useState<ViewAddress | null>(
    null,
  );
  const [deletingId, setDeletingId] = useState<string | null>(null);

  function handleOpenCreate() {
    setEditingAddress(null);
    setModalOpen(true);
  }

  function handleOpenEdit(addr: ViewAddress) {
    setEditingAddress(addr);
    setModalOpen(true);
  }

  async function handleDelete(id: string) {
    if (!confirm("Bạn có chắc chắn muốn xóa địa chỉ này?")) return;
    setDeletingId(id);
    try {
      await deleteAddressAction(id);
    } finally {
      setDeletingId(null);
    }
  }

  async function handleSetDefault(id: string) {
    await setDefaultAddressAction(id);
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold text-gray-900">
            Địa chỉ của tôi
          </h2>
          <p className="text-xs text-gray-500">
            Quản lý danh sách địa chỉ giao nhận hàng của bạn.
          </p>
        </div>
        <button
          type="button"
          onClick={handleOpenCreate}
          className="rounded-md bg-brand px-4 py-2 text-sm font-medium text-white shadow-sm transition hover:bg-brand-dark"
        >
          + Thêm địa chỉ mới
        </button>
      </div>

      {addresses.length === 0 ? (
        <div className="rounded-xl border border-dashed bg-white p-8 text-center text-gray-500">
          <p className="text-3xl">📍</p>
          <p className="mt-2 text-sm">Bạn chưa có địa chỉ nhận hàng nào.</p>
          <button
            type="button"
            onClick={handleOpenCreate}
            className="mt-3 text-sm font-medium text-brand hover:underline"
          >
            Thêm địa chỉ ngay
          </button>
        </div>
      ) : (
        <div className="grid gap-3">
          {addresses.map((addr) => (
            <div
              key={addr.id}
              className={`relative rounded-xl border p-4 transition ${
                addr.isDefault
                  ? "border-brand/40 bg-orange-50/20 shadow-xs"
                  : "border-gray-200 bg-white hover:border-gray-300"
              }`}
            >
              <div className="flex items-start justify-between">
                <div className="space-y-1">
                  <div className="flex items-center gap-2">
                    <span className="font-semibold text-gray-900">
                      {addr.recipientName}
                    </span>
                    <span className="text-xs text-gray-400">|</span>
                    <span className="text-sm text-gray-600">{addr.phone}</span>
                    {addr.isDefault && (
                      <span className="rounded bg-brand/10 px-2 py-0.5 text-[11px] font-semibold text-brand">
                        Mặc định
                      </span>
                    )}
                  </div>
                  <p className="text-sm text-gray-700">
                    {addr.street}
                    {addr.ward ? `, ${addr.ward}` : ""}
                    {addr.district ? `, ${addr.district}` : ""}
                    {`, ${addr.city}`}
                  </p>
                </div>

                <div className="flex items-center gap-2">
                  <button
                    type="button"
                    onClick={() => handleOpenEdit(addr)}
                    className="text-xs font-medium text-blue-600 hover:underline"
                  >
                    Sửa
                  </button>
                  {!addr.isDefault && (
                    <>
                      <span className="text-gray-300">·</span>
                      <button
                        type="button"
                        disabled={deletingId === addr.id}
                        onClick={() => handleDelete(addr.id)}
                        className="text-xs font-medium text-red-600 hover:underline disabled:opacity-40"
                      >
                        {deletingId === addr.id ? "Đang xóa..." : "Xóa"}
                      </button>
                      <span className="text-gray-300">·</span>
                      <button
                        type="button"
                        onClick={() => handleSetDefault(addr.id)}
                        className="rounded border border-gray-200 bg-white px-2.5 py-1 text-xs font-medium text-gray-700 hover:bg-gray-50"
                      >
                        Thiết lập mặc định
                      </button>
                    </>
                  )}
                </div>
              </div>
            </div>
          ))}
        </div>
      )}

      {modalOpen && (
        <AddressModal
          address={editingAddress}
          onClose={() => setModalOpen(false)}
        />
      )}
    </div>
  );
}
