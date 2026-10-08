## Purpose

Defines the storefront RMA flow once the faked refund is gone. The seller approves, rejects and refunds returns from
the order page and sees the refund the payment actually applied. The buyer follows the return's status. No page
claims money moved unless the payment says so.

## ADDED Requirements

### Requirement: The seller handles an order's returns from the order page

The seller's order page (`/seller/orders/{id}?tab=returns`) SHALL list the order's returns, read through the gateway
with `ListOrderReturns`. Each return SHALL show its reason, its refund amount and its status label (`Chờ duyệt`,
`Đã duyệt`, `Đã từ chối`, `Đã hoàn tiền`). The page SHALL offer these actions:

| Status | Actions |
|---|---|
| `PENDING` | `Duyệt` (approve), `Từ chối` (reject) |
| `APPROVED` | `Hoàn tiền` (refund, behind a confirmation), `Từ chối` (reject) |
| `REJECTED` | none |
| `REFUNDED` | none |

When the order was never paid online (its `paid_at` is empty, e.g. cash on delivery), an `APPROVED` return SHALL
offer no `Hoàn tiền` action. It SHALL show the message `Đơn thanh toán khi nhận hàng (COD): việc hoàn tiền được xử lý
ngoài hệ thống.` instead, and keep `Từ chối`.

Each action SHALL call only `UpdateReturnStatus` through the gateway. The storefront SHALL NOT call `RefundPayment`
for a return, and no storefront code SHALL report a refund as done without a gateway call. When an action fails, the
page SHALL show the error and the return's status as the gateway reports it.

#### Scenario: The seller approves a return from the order page

- **WHEN** the seller opens the returns tab of an order with a `PENDING` return and clicks `Duyệt`
- **THEN** the return reads `Đã duyệt` and offers `Hoàn tiền` and `Từ chối`
- **AND** a reload shows the same

#### Scenario: The seller refunds an approved return and it reads refunded

- **WHEN** the seller clicks `Hoàn tiền` on an `APPROVED` return of 200000 on a credited paid order of 500000 and
  confirms
- **THEN** the return reads `Đã hoàn tiền` with no action left
- **AND** within the settle window `GetPayment` for the order through the gateway reads a refunded amount of 200000

#### Scenario: The seller rejects a pending return

- **WHEN** the seller clicks `Từ chối` on a `PENDING` return
- **THEN** the return reads `Đã từ chối` with no action left
- **AND** the payment's refunded amount is unchanged

#### Scenario: A COD return shows that its refund is handled outside the system

- **WHEN** the seller opens the returns tab of a cash-on-delivery order, never paid online, whose return is `APPROVED`
- **THEN** the return reads `Đã duyệt` with the message `Đơn thanh toán khi nhận hàng (COD): việc hoàn tiền được xử lý
  ngoài hệ thống.` and offers `Từ chối` but no `Hoàn tiền`

#### Scenario: Refunding a return that is no longer approved shows the error

- **WHEN** the seller has the returns tab open on an `APPROVED` return, the return is rejected through the gateway
  meanwhile, and the seller clicks `Hoàn tiền` and confirms
- **THEN** the page shows an error
- **AND** after a reload the return reads `Đã từ chối`, with no refund on the payment

### Requirement: The seller sees the refund the payment actually applied

The seller's order page SHALL show the order payment's status and its refunded amount out of the payment amount,
read through the gateway with `GetPayment`. Each `REFUNDED` return SHALL show one of three states:

- `Đang xử lý hoàn tiền` (processing), until the payment lists a `RETURN` refund for that return;
- `Đã hoàn <applied amount>` once the payment lists that refund;
- `Chỉ hoàn được <applied amount>` once the payment lists that refund with an applied amount below its requested
  amount.

When the payment cannot be read, the page SHALL say so in place of the payment summary and SHALL still list the
returns and their actions. In that case each `REFUNDED` return SHALL read `Đang xử lý hoàn tiền`.

#### Scenario: A refunded return shows processing until the payment applies it

- **WHEN** `team-payment` is stopped and the seller refunds an approved return of 200000 on a credited paid order of
  500000 from the returns tab
- **THEN** the return reads `Đã hoàn tiền` and `Đang xử lý hoàn tiền`, and the payment summary says the payment could
  not be loaded
- **AND** after `team-payment` is started again and the page is reloaded within the settle window, the return reads
  `Đã hoàn 200.000₫` and the payment summary reads 200.000₫ refunded of 500.000₫

#### Scenario: A return refunded only in part says how much was refunded

- **WHEN** the seller of a credited paid order of 500000 refunds 400000 of the payment directly and then refunds an
  approved return of 300000 from the returns tab
- **THEN** after a reload within the settle window, the return reads `Chỉ hoàn được 100.000₫`
- **AND** the payment summary reads 500.000₫ refunded of 500.000₫

### Requirement: The buyer follows a return without a refund control

The buyer's order page SHALL list the order's returns and their status, read through the gateway with
`ListOrderReturns`, so they are still shown after a reload. It SHALL offer no refund control. The storefront's faked
refund helper SHALL be removed.

#### Scenario: The buyer sees the return status without a refund button

- **WHEN** the buyer requests a return on a paid order, the seller approves and refunds it, and the buyer reloads the
  order page's returns tab
- **THEN** the return reads `Đã hoàn tiền`
- **AND** the page has no `Hoàn tiền` button in any return state
