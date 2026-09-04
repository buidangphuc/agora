"use client";

import Link from "next/link";
import { useState } from "react";

import { formatPrice } from "@/components/ui/format";

export function LoanCalculatorView() {
  const [propertyPrice, setPropertyPrice] = useState<number>(3500000000); // 3.5 Tỷ
  const [loanPercent, setLoanPercent] = useState<number>(70); // 70%
  const [loanYears, setLoanYears] = useState<number>(20); // 20 năm
  const [interestRate, setInterestRate] = useState<number>(7.5); // 7.5%/năm
  const [method, setMethod] = useState<"reducing" | "equal">("reducing");

  // Calculations
  const loanAmount = (propertyPrice * loanPercent) / 100;
  const upfrontAmount = propertyPrice - loanAmount;
  const totalMonths = loanYears * 12;
  const monthlyRate = interestRate / 100 / 12;

  // Monthly principal
  const monthlyPrincipal = totalMonths > 0 ? loanAmount / totalMonths : 0;
  // First month interest
  const firstMonthInterest = loanAmount * monthlyRate;
  // First month total
  const firstMonthPayment = monthlyPrincipal + firstMonthInterest;

  // Total interest calculation
  let totalInterest = 0;
  if (method === "reducing") {
    totalInterest = (loanAmount * (totalMonths + 1) * monthlyRate) / 2;
  } else {
    // Equal payments formula
    const monthlyPayment =
      (loanAmount * monthlyRate * (1 + monthlyRate) ** totalMonths) /
      ((1 + monthlyRate) ** totalMonths - 1);
    totalInterest = monthlyPayment * totalMonths - loanAmount;
  }

  const totalCost = loanAmount + totalInterest;

  return (
    <div className="space-y-8 max-w-5xl mx-auto">
      {/* ── Header ── */}
      <div className="rounded-2xl bg-gradient-to-r from-slate-900 via-gray-900 to-red-950 p-6 md:p-8 text-white shadow-md">
        <span className="rounded-full bg-red-600/90 px-3 py-1 text-xs font-bold uppercase tracking-wider text-white shadow-xs">
          🧮 CÔNG CỤ TÀI CHÍNH BĐS
        </span>
        <h1 className="mt-3 text-2xl md:text-3xl font-black">
          Tính Lãi Suất Vay Mua Nhà Trả Góp
        </h1>
        <p className="mt-1 text-xs md:text-sm text-gray-300">
          Ước tính chính xác số tiền gốc + lãi trả hàng tháng và kế hoạch tài
          chính mua nhà an toàn
        </p>
      </div>

      {/* ── Calculator Main Grid ── */}
      <div className="grid grid-cols-1 gap-8 lg:grid-cols-12">
        {/* ── Left Form Inputs ── */}
        <div className="lg:col-span-7 rounded-2xl border border-gray-200 bg-white p-6 shadow-xs space-y-5">
          <h2 className="text-sm font-bold uppercase tracking-wider text-gray-900 border-b pb-3">
            THÔNG TIN KHOẢN VAY
          </h2>

          {/* Property Price */}
          <div>
            <div className="flex justify-between text-xs font-semibold text-gray-700 mb-1">
              <span>Giá trị bất động sản (VND):</span>
              <strong className="text-red-600 font-bold text-sm">
                {formatPrice(propertyPrice)}
              </strong>
            </div>
            <input
              type="number"
              step={50000000}
              value={propertyPrice}
              onChange={(e) => setPropertyPrice(Number(e.target.value))}
              className="w-full rounded-lg border border-gray-300 p-2.5 text-xs text-gray-900 outline-none focus:border-red-500"
            />
            <div className="flex gap-2 mt-2">
              {[2000000000, 3500000000, 5000000000, 10000000000].map((val) => (
                <button
                  key={val}
                  type="button"
                  onClick={() => setPropertyPrice(val)}
                  className={`rounded-md px-2.5 py-1 text-[11px] font-semibold transition ${
                    propertyPrice === val
                      ? "bg-red-50 text-red-600 border border-red-300"
                      : "bg-gray-100 text-gray-600 hover:bg-gray-200"
                  }`}
                >
                  {formatPrice(val)}
                </button>
              ))}
            </div>
          </div>

          {/* Loan Percentage */}
          <div>
            <div className="flex justify-between text-xs font-semibold text-gray-700 mb-1">
              <span>Tỷ lệ vay ngân hàng:</span>
              <strong className="text-gray-900">
                {loanPercent}% ({formatPrice(loanAmount)})
              </strong>
            </div>
            <input
              type="range"
              min={10}
              max={85}
              step={5}
              value={loanPercent}
              onChange={(e) => setLoanPercent(Number(e.target.value))}
              className="w-full accent-red-600 cursor-pointer"
            />
            <div className="flex justify-between text-[11px] text-gray-400 mt-1">
              <span>10%</span>
              <span>50%</span>
              <span>70% (Phổ biến)</span>
              <span>85%</span>
            </div>
          </div>

          {/* Loan Years */}
          <div>
            <div className="flex justify-between text-xs font-semibold text-gray-700 mb-1">
              <span>Thời hạn vay:</span>
              <strong className="text-gray-900">
                {loanYears} năm ({totalMonths} tháng)
              </strong>
            </div>
            <input
              type="range"
              min={5}
              max={35}
              step={5}
              value={loanYears}
              onChange={(e) => setLoanYears(Number(e.target.value))}
              className="w-full accent-red-600 cursor-pointer"
            />
            <div className="flex justify-between text-[11px] text-gray-400 mt-1">
              <span>5 năm</span>
              <span>15 năm</span>
              <span>20 năm (Khuyên dùng)</span>
              <span>35 năm</span>
            </div>
          </div>

          {/* Interest Rate */}
          <div>
            <label
              htmlFor="interest-rate"
              className="block text-xs font-semibold text-gray-700 mb-1"
            >
              Lãi suất vay ưu đãi (%/năm):
            </label>
            <input
              id="interest-rate"
              type="number"
              step={0.1}
              value={interestRate}
              onChange={(e) => setInterestRate(Number(e.target.value))}
              className="w-full rounded-lg border border-gray-300 p-2.5 text-xs text-gray-900 outline-none focus:border-red-500"
            />
          </div>

          {/* Payment Method */}
          <div>
            <div className="block text-xs font-semibold text-gray-700 mb-2">
              Phương thức tính lãi:
            </div>
            <div className="grid grid-cols-2 gap-3">
              <button
                type="button"
                onClick={() => setMethod("reducing")}
                className={`rounded-xl border p-3 text-left transition ${
                  method === "reducing"
                    ? "border-red-600 bg-red-50/50 text-red-700 ring-1 ring-red-600"
                    : "border-gray-200 hover:border-gray-300"
                }`}
              >
                <div className="font-bold text-xs">Dư nợ giảm dần</div>
                <p className="text-[11px] text-gray-500 mt-0.5">
                  Tiền lãi giảm dần theo thời gian
                </p>
              </button>

              <button
                type="button"
                onClick={() => setMethod("equal")}
                className={`rounded-xl border p-3 text-left transition ${
                  method === "equal"
                    ? "border-red-600 bg-red-50/50 text-red-700 ring-1 ring-red-600"
                    : "border-gray-200 hover:border-gray-300"
                }`}
              >
                <div className="font-bold text-xs">Trả đều hàng tháng</div>
                <p className="text-[11px] text-gray-500 mt-0.5">
                  Khoản trả cố định mỗi tháng
                </p>
              </button>
            </div>
          </div>
        </div>

        {/* ── Right Results & Summary ── */}
        <div className="lg:col-span-5 space-y-6">
          <div className="rounded-2xl border border-gray-200 bg-white p-6 shadow-xs space-y-5">
            <h2 className="text-sm font-bold uppercase tracking-wider text-gray-900 border-b pb-3">
              KẾT QUẢ ƯỚC TÍNH
            </h2>

            {/* Monthly First Payment Highlight */}
            <div className="rounded-xl bg-red-50 p-4 border border-red-200 text-center">
              <span className="text-xs text-gray-600 font-medium block">
                Số tiền trả tháng đầu tiên (Gốc + Lãi):
              </span>
              <span className="text-2xl sm:text-3xl font-black text-red-600 mt-1 block">
                {Math.round(firstMonthPayment).toLocaleString("vi-VN")} đ
              </span>
              <p className="text-[11px] text-gray-500 mt-1">
                Gốc: {Math.round(monthlyPrincipal).toLocaleString("vi-VN")} đ ·
                Lãi: {Math.round(firstMonthInterest).toLocaleString("vi-VN")} đ
              </p>
            </div>

            {/* Breakdown List */}
            <div className="space-y-3 text-xs border-t pt-4 text-gray-700">
              <div className="flex justify-between">
                <span className="text-gray-500">
                  Vốn tự có cần chuẩn bị ({100 - loanPercent}%):
                </span>
                <strong className="text-gray-900 font-bold">
                  {formatPrice(upfrontAmount)}
                </strong>
              </div>
              <div className="flex justify-between">
                <span className="text-gray-500">
                  Số tiền vay ngân hàng ({loanPercent}%):
                </span>
                <strong className="text-gray-900 font-bold">
                  {formatPrice(loanAmount)}
                </strong>
              </div>
              <div className="flex justify-between">
                <span className="text-gray-500">Tổng tiền lãi phải trả:</span>
                <strong className="text-red-600 font-bold">
                  {formatPrice(totalInterest)}
                </strong>
              </div>
              <div className="flex justify-between font-bold text-sm text-gray-900 pt-3 border-t">
                <span>Tổng gốc + lãi:</span>
                <span className="text-base text-gray-900">
                  {formatPrice(totalCost)}
                </span>
              </div>
            </div>

            {/* CTA */}
            <div className="pt-2">
              <Link
                href="/search"
                className="flex items-center justify-center rounded-xl bg-red-600 py-3 text-xs font-bold uppercase tracking-wider text-white shadow-xs hover:bg-red-700 transition"
              >
                Tìm Nhà Đất Phù Hợp Tầm Tài Chính →
              </Link>
            </div>
          </div>

          {/* Advice card */}
          <div className="rounded-xl border border-gray-100 bg-gray-50 p-4 text-xs text-gray-600 space-y-1.5">
            <p className="font-bold text-gray-800">💡 Lời khuyên chuyên gia:</p>
            <p>
              • Khoản trả góp hàng tháng không nên vượt quá{" "}
              <strong>40% - 50%</strong> tổng thu nhập của gia đình.
            </p>
            <p>
              • Chuẩn bị sẵn quỹ dự phòng tối thiểu từ 3 - 6 tháng chi tiêu
              trước khi ký hợp đồng vay vốn.
            </p>
          </div>
        </div>
      </div>
    </div>
  );
}
