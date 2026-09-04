"use client";

import Link from "next/link";
import { useState } from "react";

import { formatPrice } from "@/components/ui/format";
import { ListingGrid } from "@/features/listing/ListingGrid";
import type { ViewListing } from "@/lib/gateway/listings";

const UNIVERSITIES = [
  {
    name: "ĐH Bách Khoa - KTQD (Hà Nội)",
    kw: "Bách Khoa Hà Nội",
    count: "145 phòng",
  },
  { name: "ĐHQG Hà Nội (Cầu Giấy)", kw: "Cầu Giấy", count: "180 phòng" },
  {
    name: "ĐH Ngoại Thương - Ngoại Giao (Chùa Láng)",
    kw: "Ngoại Thương",
    count: "95 phòng",
  },
  { name: "ĐHQG TP.HCM (Làng ĐH Thủ Đức)", kw: "Thủ Đức", count: "210 phòng" },
  { name: "ĐH Bách Khoa & UEH (Quận 10)", kw: "Quận 10", count: "120 phòng" },
  {
    name: "ĐH Tôn Đức Thắng & RMIT (Quận 7)",
    kw: "Quận 7",
    count: "115 phòng",
  },
  {
    name: "ĐH HUTECH & Văn Lang (Bình Thạnh/Gò Vấp)",
    kw: "Bình Thạnh",
    count: "160 phòng",
  },
];

const PRICE_TIERS = [
  { label: "Dưới 1.5 Tr (KTX/Sleepbox)", max: 1500000 },
  { label: "1.5 Tr - 2.5 Tr (Trọ giá rẻ)", min: 1500000, max: 2500000 },
  { label: "2.5 Tr - 4.0 Tr (Khép kín, có gác)", min: 2500000, max: 4000000 },
  { label: "4.0 Tr - 6.0 Tr (Studio/CCMN)", min: 4000000, max: 6000000 },
];

export function StudentRentalView({
  initialListings,
}: {
  initialListings: ViewListing[];
}) {
  const [selectedCampus, setSelectedCampus] = useState<string>("");
  const [roomType, setRoomType] = useState<string>("all");

  // Living Cost Calculator States
  const [calcRoomPrice, setCalcRoomPrice] = useState<number>(2800000);
  const [calcElectricity, setCalcElectricity] = useState<number>(40); // 40 số
  const [calcElecPrice, setCalcElecPrice] = useState<number>(3800); // 3.8k/số
  const [calcWater, setCalcWater] = useState<number>(3); // 3 khối
  const [calcWaterPrice, setCalcWaterPrice] = useState<number>(25000); // 25k/khối
  const [calcWifi, setCalcWifi] = useState<number>(50000);
  const [calcParking, setCalcParking] = useState<number>(100000);
  const [calcRoommates, setCalcRoommates] = useState<number>(2); // 2 người ở

  const totalCost =
    calcRoomPrice +
    calcElectricity * calcElecPrice +
    calcWater * calcWaterPrice +
    calcWifi +
    calcParking;
  const costPerPerson =
    calcRoommates > 0 ? totalCost / calcRoommates : totalCost;

  // Filter listings
  const listings = initialListings.filter((l) => {
    if (roomType !== "all") {
      if (
        roomType === "ktx" &&
        !l.title.toLowerCase().includes("ký túc xá") &&
        !l.title.toLowerCase().includes("sleepbox") &&
        !l.title.toLowerCase().includes("ở ghép")
      )
        return false;
      if (
        roomType === "phong-tro" &&
        !l.title.toLowerCase().includes("phòng trọ") &&
        !l.title.toLowerCase().includes("gác")
      )
        return false;
      if (
        roomType === "studio" &&
        !l.title.toLowerCase().includes("studio") &&
        !l.title.toLowerCase().includes("căn hộ mini")
      )
        return false;
    }
    if (selectedCampus) {
      return (
        l.title.toLowerCase().includes(selectedCampus.toLowerCase()) ||
        l.description.toLowerCase().includes(selectedCampus.toLowerCase())
      );
    }
    return true;
  });

  return (
    <div className="space-y-10">
      {/* ── 1. Hero Header ── */}
      <div className="relative overflow-hidden rounded-3xl bg-gradient-to-r from-emerald-900 via-teal-900 to-slate-900 p-6 md:p-10 text-white shadow-xl">
        <div className="relative z-10 max-w-3xl space-y-3">
          <div className="flex flex-wrap items-center gap-2">
            <span className="rounded-full bg-emerald-500/90 px-3 py-1 text-xs font-bold uppercase tracking-wider text-white shadow-xs">
              🎓 BATDONGSAN SINH VIÊN
            </span>
            <span className="rounded-full bg-white/20 px-3 py-1 text-xs font-semibold backdrop-blur-xs">
              ✓ 100% Phòng Đã Xác Thực Không Lừa Cọc
            </span>
          </div>

          <h1 className="text-2xl sm:text-4xl font-black tracking-tight leading-tight">
            Tìm Phòng Trọ & Ký Túc Xá Gần Trường Đại Học{" "}
            <br className="hidden sm:inline" />
            <span className="text-emerald-400">
              Giá Rẻ, Tự Do Giờ Giấc, An Ninh Cao
            </span>
          </h1>

          <p className="text-xs sm:text-sm text-gray-200">
            Hàng ngàn phòng trọ khép kín, ký túc xá Sleepbox và căn hộ mini dành
            riêng cho sinh viên & người mới đi làm tại Hà Nội & TP. HCM.
          </p>
        </div>
      </div>

      {/* ── 2. Campus Fast Search Pills ── */}
      <section className="rounded-2xl border border-gray-200 bg-white p-5 shadow-xs space-y-3">
        <div className="flex items-center justify-between">
          <h2 className="text-sm font-bold uppercase tracking-wider text-gray-900 flex items-center gap-1.5">
            <span>🏫</span>
            <span>Tìm Phòng Trọ Theo Trường Đại Học:</span>
          </h2>
          {selectedCampus && (
            <button
              type="button"
              onClick={() => setSelectedCampus("")}
              className="text-xs text-emerald-600 font-semibold hover:underline"
            >
              ✕ Xóa lọc trường
            </button>
          )}
        </div>

        <div className="flex flex-wrap gap-2">
          {UNIVERSITIES.map((u) => (
            <button
              key={u.name}
              type="button"
              onClick={() =>
                setSelectedCampus(selectedCampus === u.kw ? "" : u.kw)
              }
              className={`flex items-center gap-1.5 rounded-xl px-3.5 py-2 text-xs font-semibold transition ${
                selectedCampus === u.kw
                  ? "bg-emerald-600 text-white shadow-xs"
                  : "bg-gray-100 text-gray-700 hover:bg-gray-200 hover:text-emerald-700"
              }`}
            >
              <span>📍 {u.name}</span>
              <span className="text-[10px] opacity-75">({u.count})</span>
            </button>
          ))}
        </div>
      </section>

      {/* ── 3. Room Type & Price Filter Bar ── */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b pb-4">
        {/* Room Type Tabs */}
        <div className="flex flex-wrap gap-2 text-xs font-bold">
          <button
            type="button"
            onClick={() => setRoomType("all")}
            className={`rounded-lg px-4 py-2 transition ${
              roomType === "all"
                ? "bg-emerald-600 text-white shadow-2xs"
                : "bg-white border border-gray-200 text-gray-700 hover:bg-gray-50"
            }`}
          >
            Tất cả phòng ({listings.length})
          </button>

          <button
            type="button"
            onClick={() => setRoomType("phong-tro")}
            className={`rounded-lg px-4 py-2 transition ${
              roomType === "phong-tro"
                ? "bg-emerald-600 text-white shadow-2xs"
                : "bg-white border border-gray-200 text-gray-700 hover:bg-gray-50"
            }`}
          >
            🏡 Phòng trọ có gác lửng
          </button>

          <button
            type="button"
            onClick={() => setRoomType("ktx")}
            className={`rounded-lg px-4 py-2 transition ${
              roomType === "ktx"
                ? "bg-emerald-600 text-white shadow-2xs"
                : "bg-white border border-gray-200 text-gray-700 hover:bg-gray-50"
            }`}
          >
            🛏️ KTX / Sleepbox cao cấp
          </button>

          <button
            type="button"
            onClick={() => setRoomType("studio")}
            className={`rounded-lg px-4 py-2 transition ${
              roomType === "studio"
                ? "bg-emerald-600 text-white shadow-2xs"
                : "bg-white border border-gray-200 text-gray-700 hover:bg-gray-50"
            }`}
          >
            🏢 Căn hộ mini / Studio
          </button>
        </div>

        {/* Quick Price Buttons */}
        <div className="flex flex-wrap items-center gap-1.5 text-xs text-gray-600">
          <span className="font-semibold text-gray-500">Mức giá:</span>
          {PRICE_TIERS.map((pt) => (
            <Link
              key={pt.label}
              href={`/search?category=cat-thue-can-ho&maxPrice=${pt.max}${pt.min ? `&minPrice=${pt.min}` : ""}`}
              className="rounded-md border border-gray-200 bg-white px-2.5 py-1 text-[11px] hover:border-emerald-500 hover:text-emerald-600 transition"
            >
              {pt.label}
            </Link>
          ))}
        </div>
      </div>

      {/* ── 4. Room Listings Grid ── */}
      <section className="space-y-4">
        <div className="flex items-center justify-between">
          <h2 className="text-base font-bold text-gray-900 flex items-center gap-2">
            <span>🏠</span>
            <span>
              Danh Sách Phòng Trọ Sinh Viên Nổi Bật ({listings.length} phòng)
            </span>
          </h2>
        </div>

        <ListingGrid
          listings={listings}
          empty="Không tìm thấy phòng trọ nào phù hợp với bộ lọc đã chọn."
        />
      </section>

      {/* ── 5. Living Cost Calculator (Công Cụ Tính Chi Phí Trọ Hàng Tháng) ── */}
      <section className="rounded-3xl border border-emerald-200 bg-gradient-to-br from-emerald-50/60 via-white to-teal-50/40 p-6 md:p-8 shadow-sm space-y-6">
        <div className="flex items-center gap-3 border-b border-emerald-100 pb-4">
          <span className="text-3xl">🧾</span>
          <div>
            <h2 className="text-lg font-black text-gray-900">
              Bảng Tính Chi Phí Trọ Hàng Tháng Thực Tế Cho Sinh Viên
            </h2>
            <p className="text-xs text-gray-500 mt-0.5">
              Dự toán chuẩn xác số tiền phòng + điện nước + internet + gửi xe
              phải đóng mỗi tháng để không bị đội chi phí
            </p>
          </div>
        </div>

        <div className="grid grid-cols-1 gap-6 lg:grid-cols-12">
          {/* Input controls */}
          <div className="lg:col-span-7 grid grid-cols-1 sm:grid-cols-2 gap-4 text-xs">
            <div>
              <label
                htmlFor="calc-room-price"
                className="font-bold text-gray-700 block mb-1"
              >
                Tiền phòng niêm yết (đ/tháng):
              </label>
              <input
                id="calc-room-price"
                type="number"
                step={100000}
                value={calcRoomPrice}
                onChange={(e) => setCalcRoomPrice(Number(e.target.value))}
                className="w-full rounded-lg border border-gray-300 p-2 text-xs font-semibold text-gray-900 outline-none focus:border-emerald-500"
              />
            </div>

            <div>
              <label
                htmlFor="calc-roommates"
                className="font-bold text-gray-700 block mb-1"
              >
                Số người ở chung (chia sẻ tiền phòng):
              </label>
              <select
                id="calc-roommates"
                value={calcRoommates}
                onChange={(e) => setCalcRoommates(Number(e.target.value))}
                className="w-full rounded-lg border border-gray-300 p-2 text-xs font-semibold text-gray-900 outline-none focus:border-emerald-500"
              >
                <option value={1}>1 người (Ở 1 mình)</option>
                <option value={2}>2 người (Ở ghép đôi)</option>
                <option value={3}>3 người</option>
                <option value={4}>4 người</option>
              </select>
            </div>

            <div>
              <label
                htmlFor="calc-electricity"
                className="font-bold text-gray-700 block mb-1"
              >
                Tiền điện: {calcElectricity} số x {calcElecPrice} đ/số ={" "}
                <strong>
                  {Math.round(calcElectricity * calcElecPrice).toLocaleString(
                    "vi-VN",
                  )}{" "}
                  đ
                </strong>
              </label>
              <input
                id="calc-electricity"
                type="range"
                min={10}
                max={150}
                value={calcElectricity}
                onChange={(e) => setCalcElectricity(Number(e.target.value))}
                className="w-full accent-emerald-600 cursor-pointer"
              />
            </div>

            <div>
              <label
                htmlFor="calc-water"
                className="font-bold text-gray-700 block mb-1"
              >
                Tiền nước: {calcWater} khối x {calcWaterPrice} đ/khối ={" "}
                <strong>
                  {Math.round(calcWater * calcWaterPrice).toLocaleString(
                    "vi-VN",
                  )}{" "}
                  đ
                </strong>
              </label>
              <input
                id="calc-water"
                type="range"
                min={1}
                max={10}
                value={calcWater}
                onChange={(e) => setCalcWater(Number(e.target.value))}
                className="w-full accent-emerald-600 cursor-pointer"
              />
            </div>

            <div>
              <label
                htmlFor="calc-wifi"
                className="font-bold text-gray-700 block mb-1"
              >
                Phí Wifi / Internet (đ/tháng):
              </label>
              <input
                id="calc-wifi"
                type="number"
                step={10000}
                value={calcWifi}
                onChange={(e) => setCalcWifi(Number(e.target.value))}
                className="w-full rounded-lg border border-gray-300 p-2 text-xs text-gray-900 outline-none focus:border-emerald-500"
              />
            </div>

            <div>
              <label
                htmlFor="calc-parking"
                className="font-bold text-gray-700 block mb-1"
              >
                Phí gửi xe máy (đ/xe/tháng):
              </label>
              <input
                id="calc-parking"
                type="number"
                step={10000}
                value={calcParking}
                onChange={(e) => setCalcParking(Number(e.target.value))}
                className="w-full rounded-lg border border-gray-300 p-2 text-xs text-gray-900 outline-none focus:border-emerald-500"
              />
            </div>
          </div>

          {/* Results column */}
          <div className="lg:col-span-5 rounded-2xl bg-emerald-900 p-6 text-white space-y-4 flex flex-col justify-between">
            <div>
              <span className="text-xs uppercase tracking-wider text-emerald-300 font-bold block">
                TỔNG CHI PHÍ THỰC TẾ HÀNG THÁNG
              </span>
              <div className="mt-2 flex items-baseline gap-2">
                <span className="text-3xl font-black text-white">
                  {Math.round(totalCost).toLocaleString("vi-VN")} đ
                </span>
                <span className="text-xs text-emerald-200">/ phòng</span>
              </div>

              {calcRoommates > 1 && (
                <div className="mt-3 rounded-xl bg-white/10 p-3 text-xs border border-white/20">
                  <span className="text-emerald-300 block">
                    Mỗi sinh viên chỉ cần trả:
                  </span>
                  <strong className="text-xl font-bold text-white block mt-0.5">
                    {Math.round(costPerPerson).toLocaleString("vi-VN")} đ /
                    người
                  </strong>
                </div>
              )}
            </div>

            <div className="text-[11px] text-emerald-200/80 border-t border-white/10 pt-3">
              💡 Tiết kiệm: Khi ở ghép 2 - 3 bạn, chi phí phòng và điện nước
              giảm hơn <strong>50% - 65%</strong> mỗi tháng.
            </div>
          </div>
        </div>
      </section>

      {/* ── 6. Student Safety & Anti-Scam Guidelines ── */}
      <section className="rounded-2xl border border-gray-200 bg-white p-6 shadow-xs space-y-3">
        <h3 className="font-bold text-sm text-gray-900 flex items-center gap-2">
          <span>🛡️</span>
          <span>Cẩm Nang Thuê Trọ An Toàn Cho Tân Sinh Viên</span>
        </h3>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4 text-xs text-gray-600">
          <div className="rounded-xl bg-gray-50 p-4 border border-gray-100 space-y-1">
            <strong className="text-gray-900 block">
              1. Không chuyển cọc khi chưa xem phòng
            </strong>
            <p>
              Tuyệt đối không chuyển khoản giữ chỗ nếu chưa trực tiếp đến xem
              phòng thực tế và gặp mặt chính chủ nhà.
            </p>
          </div>
          <div className="rounded-xl bg-gray-50 p-4 border border-gray-100 space-y-1">
            <strong className="text-gray-900 block">
              2. Đọc kỹ hợp đồng & biên bản bàn giao
            </strong>
            <p>
              Kiểm tra kỹ chỉ số công tơ điện nước, thời hạn cọc và các chi phí
              phát sinh trước khi đặt bút ký hợp đồng.
            </p>
          </div>
          <div className="rounded-xl bg-gray-50 p-4 border border-gray-100 space-y-1">
            <strong className="text-gray-900 block">
              3. Kiểm tra an ninh & PCCC
            </strong>
            <p>
              Ưu tiên các tòa nhà có khóa cổng vân tay, camera an ninh 24/7,
              thang thoát hiểm và thiết bị PCCC đầy đủ.
            </p>
          </div>
        </div>
      </section>
    </div>
  );
}
