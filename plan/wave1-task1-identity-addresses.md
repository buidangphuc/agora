# [W1-T1] Shipping addresses (team-identity)

## Role
SE

## Objective
Buyer CRUD địa chỉ giao hàng + chọn 1 địa chỉ mặc định, gate bằng scope buyer.

## Write-set (EXCLUSIVE)
- team-identity (edit — handler/service/repo + migration `addresses` + vendored proto/generated)

## Read-only dependencies
- platform-core/packages/proto (Address RPC — định ở W0-T1)
- 00-spec.md §Contracts, §Conventions

## Contracts you implement
- Address{id,buyer_id,name,phone,line,ward,district,city,is_default}
- RPC AddAddress/UpdateAddress/DeleteAddress/ListAddresses/SetDefaultAddress (scope: buyer)

## Acceptance criteria
- [ ] CRUD hoạt động, RequireScopes(buyer)
- [ ] Chỉ 1 địa chỉ is_default=true tại một thời điểm (set mới → bỏ default cũ)
- [ ] Xoá địa chỉ default → không còn default treo; migration `addresses` chạy sạch
- [ ] ≥3 test (happy + default-uniqueness + delete-default)

## Verify
docker run go test ./... trong team-identity

## Out of scope
- Không đụng auth/JWT signing; không route ở gateway (wave 3); không sửa proto
