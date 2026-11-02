## Decisions

- Layout branch order: no principal -> `redirectGuestToLoginFromHeader`; `hasSellerAccess` -> normal layout; `status === "restricted"` -> existing `SellerAccessDenied` restricted 403; else `SellerOnboarding`.
- The seller app shows one current terms version from a constant (`SELLER_TERMS_VERSION`, build/runtime env of team-frontend-seller) that must equal the first entry of identity's `SELLER_TERMS_VERSIONS`; a mismatch surfaces as the INVALID_ARGUMENT error line.
- The access token in the session claims carries only `status: "restricted"`, so suspended/banned is not known at render. Suspend and ban bulk-revoke sessions, so such a buyer's `BecomeSeller` answers UNAUTHENTICATED and goes to login; PERMISSION_DENIED arises only for an account restricted after page load. On PERMISSION_DENIED the action returns `{ error: "denied" }` and the onboarding component swaps to the denied notice; no cookie is changed.
- Action outcomes: success -> set `seller_session` (same cookie options as login) and `redirect("/seller/new")`; PERMISSION_DENIED -> `denied`; UNAUTHENTICATED -> `redirectToLogin` with `returnUrl` equal to the path the layout was rendered on (`/seller/new` for onboarding); INVALID_ARGUMENT and other failures -> `error` with a visible line; an empty token is an error; no cookie on any failure.
- No startup check ties `SELLER_TERMS_VERSION` to identity's list (accepted assumption); a mismatch shows the INVALID_ARGUMENT error line.
- `SellerAccessDenied` keeps only the restricted branch; its "Không có quyền" / "Cần tài khoản Người Bán" branch is removed.
