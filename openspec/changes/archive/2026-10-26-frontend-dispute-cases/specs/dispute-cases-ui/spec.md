## ADDED Requirements

### Requirement: A buyer escalates an escalable return to Agora

On the buyer's order detail the returns tab SHALL show the button "Khiếu nại lên Agora" on a return card if and only if the return has `canEscalate` true and no dispute with that `return_id` is OPEN or INVESTIGATING. Submitting the modal SHALL call `CreateDispute` once with `order_id`, `defendant_id` = the order's `sellerId`, `return_id` and the trimmed reason, and SHALL show the new dispute with status "Đang chờ xử lý". The reason SHALL be required and at most 1000 characters, checked before any call. A server refusal (`FAILED_PRECONDITION`, `ALREADY_EXISTS`, `PERMISSION_DENIED`, `NOT_FOUND`, `INVALID_ARGUMENT`, `UNAVAILABLE` or a 5 s timeout) SHALL be shown in the modal as the matching Vietnamese message from the proposal copy table, SHALL keep the typed reason in the field and SHALL NOT show a dispute. A buyer MAY escalate the same return again after its dispute is RESOLVED or REJECTED while `canEscalate` is true.

#### Scenario: Button is shown on an escalable return
- **WHEN** the buyer opens the returns tab of an order whose return has `canEscalate` true and no open dispute
- **THEN** the return card shows one "Khiếu nại lên Agora" button
- **VERIFIED BY**: `team-frontend/src/features/order/ReturnRequestSection.test.tsx` `shows the escalate button when canEscalate and no open dispute`

#### Scenario: Button is hidden when the return cannot be escalated
- **WHEN** the return has `canEscalate` false
- **THEN** the return card shows no "Khiếu nại lên Agora" button
- **VERIFIED BY**: `team-frontend/src/features/order/ReturnRequestSection.test.tsx` `hides the escalate button when canEscalate is false`

#### Scenario: Button is hidden while a dispute is open for the return
- **WHEN** `canEscalate` is true and a dispute with that `return_id` is OPEN or INVESTIGATING
- **THEN** the return card shows no escalate button and the dispute section shows that dispute
- **VERIFIED BY**: `team-frontend/src/features/order/ReturnRequestSection.test.tsx` `hides the escalate button while a dispute is open`

#### Scenario: A closed dispute does not hide the button forever
- **WHEN** `canEscalate` is true and the return's disputes are one REJECTED and one RESOLVED, none OPEN or INVESTIGATING
- **THEN** the return card shows the escalate button again
- **VERIFIED BY**: `team-frontend/src/features/order/ReturnRequestSection.test.tsx` `shows the escalate button after a closed dispute`

#### Scenario: Escalation creates a dispute
- **WHEN** the buyer enters the reason "Người bán từ chối vô lý" and submits
- **THEN** `CreateDispute` is called exactly once with `orderId`, `defendantId` equal to the order's `sellerId`, the `returnId` and that reason, and the dispute section shows one entry with "Đang chờ xử lý"
- **VERIFIED BY**: `team-frontend/src/features/dispute/actions.test.ts` `createDisputeAction sends order, seller and return ids`; `team-frontend/src/features/order/ReturnRequestSection.test.tsx` `shows the created dispute as Đang chờ xử lý`

#### Scenario: Blank reason is refused locally
- **WHEN** the buyer submits a reason that is empty or only spaces
- **THEN** the modal shows "Vui lòng nhập lý do khiếu nại." and `CreateDispute` is called zero times
- **VERIFIED BY**: `team-frontend/src/features/dispute/EscalateDisputeModal.test.tsx` `refuses a blank reason without a call`

#### Scenario: Reason at the length limit
- **WHEN** the buyer submits a reason of exactly 1000 characters, then one of 1001 characters
- **THEN** the 1000-character reason is sent to `CreateDispute` and the 1001-character reason shows "Lý do tối đa 1000 ký tự." with zero calls
- **VERIFIED BY**: `team-frontend/src/features/dispute/EscalateDisputeModal.test.tsx` `accepts 1000 characters and refuses 1001`

#### Scenario: Server says the return cannot be escalated
- **WHEN** `CreateDispute` answers `FAILED_PRECONDITION`
- **THEN** the modal shows "Yêu cầu trả hàng này chưa thể khiếu nại." and no dispute entry is added
- **VERIFIED BY**: `team-frontend/src/features/dispute/actions.test.ts` `maps FAILED_PRECONDITION to the cannot-escalate message`; `team-frontend/src/features/dispute/EscalateDisputeModal.test.tsx` `shows a refusal message, keeps the reason and adds no entry`

#### Scenario: A dispute already exists for the return
- **WHEN** `CreateDispute` answers `ALREADY_EXISTS`
- **THEN** the action returns "Yêu cầu trả hàng này đã có khiếu nại đang xử lý." and revalidates `/account/orders/<id>` so the order's disputes are reloaded, and the modal shows that message
- **VERIFIED BY**: `team-frontend/src/features/dispute/actions.test.ts` `maps ALREADY_EXISTS and revalidates the order`; `team-frontend/src/features/dispute/EscalateDisputeModal.test.tsx` `shows a refusal message, keeps the reason and adds no entry`

#### Scenario: Buyer is not a party
- **WHEN** `CreateDispute` answers permission denied (`PERMISSION_DENIED`) for an order the caller is not a party to
- **THEN** the modal shows "Bạn không có quyền khiếu nại đơn hàng này." and no dispute entry is added
- **VERIFIED BY**: `team-frontend/src/features/dispute/actions.test.ts` `maps PERMISSION_DENIED`; `team-frontend/src/features/dispute/EscalateDisputeModal.test.tsx` `shows a refusal message, keeps the reason and adds no entry`

#### Scenario: Return or order not found
- **WHEN** `CreateDispute` answers `NOT_FOUND` (unknown return, a return of another order, or an order the caller cannot see)
- **THEN** the action returns "Không tìm thấy đơn hàng hoặc yêu cầu trả hàng này." without the server error text, and no dispute entry is added
- **VERIFIED BY**: `team-frontend/src/features/dispute/actions.test.ts` `maps NOT_FOUND to the not-found message`

#### Scenario: Server refuses the defendant
- **WHEN** `CreateDispute` answers `INVALID_ARGUMENT` (for example the order's seller changed under a stale page)
- **THEN** the action returns "Không gửi được khiếu nại. Vui lòng tải lại trang và thử lại." without the server error text, and no dispute entry is added
- **VERIFIED BY**: `team-frontend/src/features/dispute/actions.test.ts` `maps INVALID_ARGUMENT to the reload message`

#### Scenario: Gateway outage on create
- **WHEN** `CreateDispute` fails with `UNAVAILABLE` or times out after 5 s
- **THEN** the action returns "Không gửi được khiếu nại. Vui lòng thử lại sau." without the server error text, and the modal shows it with the reason still in the field
- **VERIFIED BY**: `team-frontend/src/features/dispute/actions.test.ts` `maps outage and never forwards err.message`; `team-frontend/src/features/dispute/EscalateDisputeModal.test.tsx` `shows a refusal message, keeps the reason and adds no entry`

#### Scenario: Double click sends one request
- **WHEN** the buyer clicks "Gửi khiếu nại" twice before the first call returns
- **THEN** `CreateDispute` is called exactly once and the submit button is disabled until it returns
- **VERIFIED BY**: `team-frontend/src/features/dispute/EscalateDisputeModal.test.tsx` `is single-flight`

#### Scenario: Buyer escalates a rejected return in the browser
- **WHEN** a buyer with a REJECTED return whose `can_escalate` is true taps "Khiếu nại lên Agora", types a reason and submits
- **THEN** the order detail shows a dispute with "Đang chờ xử lý" and the button is gone
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/dispute_cases_ui.feature` `Buyer escalates a rejected return`

### Requirement: The order detail lists the order's disputes

The returns tab SHALL list the disputes returned by `ListOrderDisputes` for the order, newest first as the server returns them, each with status text, ruling text when set, the linked return and the resolution text. A failure of that call SHALL hide only the dispute section content behind "Không tải được danh sách khiếu nại." and SHALL leave the order, items and returns rendered. Dispute text SHALL be rendered as text, never as HTML.

#### Scenario: Disputes are listed with status and ruling
- **WHEN** the server returns two disputes, one RESOLVED with ruling BUYER_FAVOUR and resolution "Hoàn tiền", one REJECTED
- **THEN** the section shows two entries in the returned order, the first with "Đã giải quyết", "Người mua thắng" and "Hoàn tiền", the second with "Đã từ chối" and no ruling
- **VERIFIED BY**: `team-frontend/src/features/dispute/OrderDisputeList.test.tsx` `lists disputes with status and ruling in server order`

#### Scenario: Order has no disputes
- **WHEN** `ListOrderDisputes` returns an empty list
- **THEN** the section shows "Chưa có khiếu nại"
- **VERIFIED BY**: `team-frontend/src/features/dispute/OrderDisputeList.test.tsx` `shows the empty state`

#### Scenario: Dispute list failure leaves the order page intact
- **WHEN** `ListOrderDisputes` fails with `UNAVAILABLE`, or answers `PERMISSION_DENIED`
- **THEN** the order page still renders the items and the returns and the dispute section shows "Không tải được danh sách khiếu nại." without server error text
- **VERIFIED BY**: `team-frontend/src/lib/gateway/disputes.test.ts` `listOrderDisputes maps failures to an error result`

#### Scenario: Markup in a dispute is shown as text
- **WHEN** a dispute reason is `<img src=x onerror=alert(1)>`
- **THEN** the section shows that exact string as text and renders no `img` element
- **VERIFIED BY**: `team-frontend/src/features/dispute/OrderDisputeList.test.tsx` `renders dispute text as text`

### Requirement: The Khiếu nại tab is admin-only

`/admin/cases?tab=disputes` SHALL redirect to `/login` (through `redirectGuestToLogin`) when there is no principal or token, SHALL render the 403 `Result` titled "Cần tài khoản Quản trị" when the principal lacks the `admin` scope, and SHALL call neither `ListDisputes` nor `GetDispute` in either case. A gateway `PermissionDenied` SHALL render the same 403 and `Unauthenticated` SHALL redirect to `/login`.

#### Scenario: Anonymous visitor on the dispute tab is redirected to login
- **WHEN** a request with no session opens `/admin/cases?tab=disputes`
- **THEN** the page redirects to `/login` and `ListDisputes` is called zero times
- **VERIFIED BY**: `team-frontend/src/app/(shop)/admin/cases/page.test.tsx` `redirects an anonymous visitor on the disputes tab`

#### Scenario: Non-admin sees the 403 result on the dispute tab
- **WHEN** a signed-in buyer without the `admin` scope opens `/admin/cases?tab=disputes`
- **THEN** the page renders status 403 with "Cần tài khoản Quản trị" and `ListDisputes` is called zero times
- **VERIFIED BY**: `team-frontend/src/app/(shop)/admin/cases/page.test.tsx` `renders 403 on the disputes tab for a non-admin`

#### Scenario: Gateway refusal and expired session
- **WHEN** the principal has the `admin` scope and `ListDisputes` answers `PermissionDenied`, then `Unauthenticated`
- **THEN** the first render shows the 403 result and no rows, the second redirects to `/login`
- **VERIFIED BY**: `team-frontend/src/app/(shop)/admin/cases/page.test.tsx` `maps gateway PermissionDenied and Unauthenticated on the disputes tab`

#### Scenario: A buyer opening the tab in the browser is refused
- **WHEN** a signed-in buyer opens `/admin/cases?tab=disputes` in the browser
- **THEN** the page shows "Cần tài khoản Quản trị" and no dispute list
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/dispute_cases_ui.feature` `A buyer cannot open the dispute tab`

### Requirement: Admins list disputes oldest first, filtered by status

For an admin the tab SHALL call `ListDisputes` once per render with the `status` from the URL (default OPEN) and `page.pageSize = 20`, SHALL render the rows in the returned order (oldest first), and SHALL render a "Trang sau" link only when `next_cursor` is non-empty; the link SHALL keep the status filter and set the cursor. An unknown `status` value SHALL fall back to OPEN. A failed call SHALL show "Không tải được danh sách khiếu nại. Vui lòng thử lại sau." and no rows.

#### Scenario: Default list is OPEN, 20 per page
- **WHEN** an admin opens `/admin/cases?tab=disputes` with no filter
- **THEN** `ListDisputes` is called once with `status = OPEN`, `pageSize = 20` and an empty cursor
- **VERIFIED BY**: `team-frontend/src/lib/gateway/disputes.test.ts` `listDisputes defaults to OPEN and page size 20`

#### Scenario: Status filter and server order
- **WHEN** an admin opens the tab with `status=investigating` and the server returns three disputes with ascending `created_at`
- **THEN** the call carries `status = INVESTIGATING` and the rows appear in the returned order
- **VERIFIED BY**: `team-frontend/src/features/admin/DisputeCasesPanel.test.tsx` `keeps the server order and applies the status filter`

#### Scenario: Non-admin token is denied the list
- **WHEN** `ListDisputes` answers `PermissionDenied` for a token the page believed to be admin
- **THEN** `listDisputes` returns the `forbidden` result with no rows and no server error text (the page renders it as 403, see "Gateway refusal and expired session")
- **VERIFIED BY**: `team-frontend/src/lib/gateway/disputes.test.ts` `listDisputes maps PermissionDenied to forbidden`

#### Scenario: Unknown status falls back to OPEN
- **WHEN** the URL has `status=bogus`
- **THEN** the call carries `status = OPEN`
- **VERIFIED BY**: `team-frontend/src/lib/gateway/disputes.test.ts` `ignores an unknown status`

#### Scenario: Next page link carries the cursor
- **WHEN** the response has `next_cursor` "c2" under filter `status=open`
- **THEN** the panel renders a "Trang sau" link to `/admin/cases?tab=disputes&status=open&cursor=c2`, and no link when `next_cursor` is empty
- **VERIFIED BY**: `team-frontend/src/features/admin/DisputeCasesPanel.test.tsx` `builds the next link from the filter and cursor`

#### Scenario: Admin tab has no disputes
- **WHEN** `ListDisputes` returns an empty list
- **THEN** the panel shows "Không có khiếu nại nào"
- **VERIFIED BY**: `team-frontend/src/features/admin/DisputeCasesPanel.test.tsx` `shows the empty state`

#### Scenario: Load failure
- **WHEN** `ListDisputes` fails with `UNAVAILABLE`
- **THEN** the panel shows "Không tải được danh sách khiếu nại. Vui lòng thử lại sau." with no rows and no server error text
- **VERIFIED BY**: `team-frontend/src/lib/gateway/disputes.test.ts` `listDisputes maps UNAVAILABLE to an error result`

#### Scenario: Deep link opens one dispute
- **WHEN** an admin opens `/admin/cases?tab=disputes&dispute=d-1`, then a dispute id the server answers `NOT_FOUND` for
- **THEN** `GetDispute` is called with the id; the first shows that dispute's detail (reason, status and its actions) and the second shows "Không tìm thấy khiếu nại này."
- **VERIFIED BY**: `team-frontend/src/app/(shop)/admin/cases/page.test.tsx` `deep link renders the dispute detail`; `team-frontend/src/lib/gateway/disputes.test.ts` `getDispute maps NOT_FOUND to not_found`

#### Scenario: Row detail and unsafe evidence URL
- **WHEN** a dispute has evidence URLs "https://x.test/a.jpg" and "javascript:alert(1)"
- **THEN** the first is an anchor with `rel="noopener noreferrer"` and the second is plain text with no anchor
- **VERIFIED BY**: `team-frontend/src/features/admin/DisputeCasesPanel.test.tsx` `links only http and https evidence`

### Requirement: Admin actions follow the dispute state machine

The panel SHALL offer only the transitions the server allows: for OPEN "Bắt đầu điều tra" and "Từ chối"; for INVESTIGATING "Cập nhật ghi chú", "Giải quyết" and "Từ chối"; for RESOLVED and REJECTED no action. After a successful action the panel SHALL show that dispute's detail at `/admin/cases?tab=disputes&dispute=<id>` (loaded through `GetDispute`) with its new status and the actions of that status, whatever the list filter was. Each action SHALL call `ResolveDispute` once with the target status, the note as `resolution` and, for "Giải quyết" on a dispute with a `return_id`, the chosen ruling. "Giải quyết" on a return-linked dispute SHALL be refused locally until a ruling is chosen. Choosing "Người mua thắng" SHALL first show the confirmation "Xác nhận phán quyết người mua thắng? Yêu cầu trả hàng sẽ được duyệt; nếu đơn đã thanh toán trực tuyến, người mua sẽ được hoàn tiền.", and the call SHALL be sent only after "Xác nhận" (the text does not promise a refund, since a COD order stops at APPROVED). A dispute without `return_id` SHALL send no ruling.

#### Scenario: Actions offered per status
- **WHEN** the panel renders one dispute each in OPEN, INVESTIGATING, RESOLVED and REJECTED
- **THEN** OPEN shows "Bắt đầu điều tra" and "Từ chối" but no "Giải quyết"; INVESTIGATING shows "Cập nhật ghi chú", "Giải quyết" and "Từ chối"; RESOLVED and REJECTED show no action
- **VERIFIED BY**: `team-frontend/src/features/dispute/allowedActions.test.ts` `offers the server state machine only`

#### Scenario: Start investigating
- **WHEN** an admin clicks "Bắt đầu điều tra" on an OPEN dispute
- **THEN** `ResolveDispute` is called once with `status = INVESTIGATING`, and the panel shows that dispute's detail at `?tab=disputes&dispute=<id>` with "Đang điều tra" and the actions "Cập nhật ghi chú", "Giải quyết" and "Từ chối"
- **VERIFIED BY**: `team-frontend/src/features/dispute/actions.test.ts` `resolveDisputeAction moves OPEN to INVESTIGATING`; `team-frontend/src/features/admin/DisputeCasesPanel.test.tsx` `shows the dispute detail with its new status after an action`

#### Scenario: Ruling is required for a return-linked dispute
- **WHEN** an admin clicks "Giải quyết" on an INVESTIGATING dispute that has a `return_id` without choosing a ruling
- **THEN** the panel shows "Vui lòng chọn phán quyết." and `ResolveDispute` is called zero times
- **VERIFIED BY**: `team-frontend/src/features/admin/DisputeCasesPanel.test.tsx` `requires a ruling for a return-linked dispute`

#### Scenario: Buyer-favour needs confirmation
- **WHEN** an admin picks "Người mua thắng" and clicks "Giải quyết"
- **THEN** the panel shows "Xác nhận phán quyết người mua thắng? Yêu cầu trả hàng sẽ được duyệt; nếu đơn đã thanh toán trực tuyến, người mua sẽ được hoàn tiền." and `ResolveDispute` is called only after "Xác nhận", with `status = RESOLVED` and `ruling = BUYER_FAVOUR`; "Hủy" sends nothing
- **VERIFIED BY**: `team-frontend/src/features/admin/DisputeCasesPanel.test.tsx` `confirms a buyer-favour ruling before sending`

#### Scenario: Seller-favour and reject send the right payload
- **WHEN** an admin resolves with "Người bán thắng", then rejects another dispute with a note
- **THEN** the first call has `status = RESOLVED`, `ruling = SELLER_FAVOUR`; the second has `status = REJECTED` and no ruling
- **VERIFIED BY**: `team-frontend/src/features/dispute/actions.test.ts` `sends seller-favour and reject payloads`

#### Scenario: Dispute without a return takes no ruling
- **WHEN** an admin resolves an INVESTIGATING dispute with no `return_id`
- **THEN** no ruling control is shown and `ResolveDispute` is called with `status = RESOLVED` and no ruling
- **VERIFIED BY**: `team-frontend/src/features/admin/DisputeCasesPanel.test.tsx` `omits the ruling for a dispute without a return`

#### Scenario: Server refuses the transition
- **WHEN** `ResolveDispute` answers `FAILED_PRECONDITION`, or `INVALID_ARGUMENT`
- **THEN** the action returns "Không thể chuyển khiếu nại sang trạng thái này." without the server error text, and the panel shows it while the dispute keeps its status and actions
- **VERIFIED BY**: `team-frontend/src/features/dispute/actions.test.ts` `maps a refused transition`; `team-frontend/src/features/admin/DisputeCasesPanel.test.tsx` `shows an action error and keeps the status`

#### Scenario: Audit service is down
- **WHEN** `ResolveDispute` answers `UNAVAILABLE`
- **THEN** the action returns "Hệ thống kiểm toán tạm thời không khả dụng. Chưa có thay đổi nào được lưu.", and the panel shows it while the dispute keeps its status and actions
- **VERIFIED BY**: `team-frontend/src/features/dispute/actions.test.ts` `maps UNAVAILABLE to the audit-down message`; `team-frontend/src/features/admin/DisputeCasesPanel.test.tsx` `shows an action error and keeps the status`

#### Scenario: Resolve times out
- **WHEN** `ResolveDispute` does not answer within 5 s
- **THEN** the action returns "Chưa rõ thao tác đã được lưu hay chưa. Vui lòng tải lại trang để xem trạng thái hiện tại." (not the audit-down text) and revalidates `/admin/cases`
- **VERIFIED BY**: `team-frontend/src/features/dispute/actions.test.ts` `maps a resolve timeout to the unknown-state message`

#### Scenario: Non-admin cannot call the action
- **WHEN** the resolve server action runs for a principal without the `admin` scope
- **THEN** it returns a refusal and `ResolveDispute` is called zero times
- **VERIFIED BY**: `team-frontend/src/features/dispute/actions.test.ts` `refuses a non-admin before calling the gateway`

#### Scenario: Double click on a resolve sends one request
- **WHEN** an admin clicks "Xác nhận" twice before the call returns
- **THEN** `ResolveDispute` is called exactly once
- **VERIFIED BY**: `team-frontend/src/features/admin/DisputeCasesPanel.test.tsx` `is single-flight`

#### Scenario: Admin investigates and rules for the buyer in the browser
- **WHEN** an admin opens the OPEN dispute created by the buyer at `/admin/cases?tab=disputes&dispute=<id>`, clicks "Bắt đầu điều tra", and on the same detail (now "Đang điều tra") picks "Người mua thắng", clicks "Giải quyết" and "Xác nhận"
- **THEN** the dispute detail shows "Đã giải quyết" and, after the buyer reloads the order, the return shows "Đã hoàn tiền" and the dispute shows "Người mua thắng"
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/dispute_cases_ui.feature` `Admin rules for the buyer and the return is refunded`

#### Scenario: Seller-favour ruling moves no return in the browser
- **WHEN** an admin opens an INVESTIGATING return-linked dispute at `/admin/cases?tab=disputes&dispute=<id>` and resolves it with "Người bán thắng"
- **THEN** the buyer's order shows the dispute "Đã giải quyết" with "Người bán thắng" and the return status text is unchanged ("Đã từ chối")
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/dispute_cases_ui.feature` `Seller-favour ruling leaves the return unchanged`

#### Scenario: Admin cannot skip investigation in the browser
- **WHEN** an admin opens an OPEN dispute in the browser
- **THEN** the row shows "Bắt đầu điều tra" and "Từ chối" and no "Giải quyết" button
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/dispute_cases_ui.feature` `An open dispute cannot be resolved directly`
