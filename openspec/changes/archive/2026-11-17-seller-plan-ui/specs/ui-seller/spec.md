# ui-seller Specification

## MODIFIED Requirements

### Requirement: Wallet, plans, ads and bundles use Server Actions with the shared mutation contract

`/seller/wallet`, `/seller/plans`, `/seller/ads` and `/seller/bundles` SHALL be server pages composed of
`Statistic`, `Card`, `Table`, `Pagination` and `FormItem`-based forms. Payout, subscribe, cancel-subscription,
create-ad-campaign and create-bundle SHALL be Server Actions returning `{ ok: boolean; error?: string; data?: T }`
that call `revalidatePath` for their route; their client buttons SHALL show a pending state, be disabled while
pending and when the action is not applicable (zero balance, current plan, fewer than 2 bundle items), and show a
success or error toast. Payout SHALL require a confirm `Modal`; subscribe and cancel-subscription SHALL require a
confirm `Modal` that states the charge or the period end. Plans SHALL mark the current plan with a `Tag`.

#### Scenario: Payout confirms, pends and toasts

- **WHEN** a seller with a positive balance activates "Rút tiền" and confirms
- **THEN** the confirm button is pending then disabled, a success toast appears and the balance and ledger
  refresh

#### Scenario: Payout is disabled without balance

- **WHEN** the balance is 0
- **THEN** the payout `Button` is disabled with `aria-disabled="true"` and explains why

#### Scenario: Current plan cannot be re-subscribed

- **WHEN** the seller views `/seller/plans`
- **THEN** the current plan shows a "Gói hiện tại" `Tag` and a disabled button, other plans show an enabled
  "Nâng cấp" button above the current tier and an enabled "Hạ gói" button below it

#### Scenario: Action errors use the error field

- **WHEN** `createBundleAction` returns `{ ok: false, error: "Chọn ít nhất 2 sản phẩm cho combo." }`
- **THEN** the field-level `FormItem` or an `Alert` shows that message and an error toast appears

#### Scenario: Bundle creation succeeds

- **WHEN** the seller creates a valid bundle
- **THEN** a success toast appears, the form resets and the bundle appears in the list after revalidation
