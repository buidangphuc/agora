# return-read-access Specification

## Purpose
Service principals with order.read can read return requests.

## Requirements

### Requirement: GetReturnRequest accepts a service principal with order.read

`OrderService.GetReturnRequest` SHALL return the return request, including `can_escalate`, to a principal of type service that holds scope `order.read`, in addition to the return's buyer, the return's seller and an admin. `UpdateReturnStatus` SHALL keep refusing every principal except the return's seller and an admin. No other caller SHALL read another user's return.

#### Scenario: Return read: Service principal with order.read reads a return
- **WHEN** `service-team-engagement` (type service, scope `order.read`) calls GetReturnRequest for a return of any buyer
- **THEN** the call returns OK with the return and its can_escalate value
- **VERIFIED BY**: team-order `internal/handler` `TestGetReturnRequestServicePrincipalOrderRead`

#### Scenario: Return read: Service principal without order.read is denied
- **WHEN** a principal of type service with scope `payment.read` only calls GetReturnRequest
- **THEN** the call returns `PERMISSION_DENIED`
- **VERIFIED BY**: team-order `internal/handler` `TestGetReturnRequestServiceWithoutScopeDenied`

#### Scenario: Return read: A user principal with the scope string is not a service
- **WHEN** a principal of type user carrying scope `order.read` but who is neither buyer, seller nor admin calls GetReturnRequest
- **THEN** the call returns `PERMISSION_DENIED`
- **VERIFIED BY**: team-order `internal/handler` `TestGetReturnRequestUserWithOrderReadScopeDenied`

#### Scenario: Return read: Another user is still denied
- **WHEN** a signed-in user who is neither the buyer nor the seller of the return calls GetReturnRequest
- **THEN** the call returns `PERMISSION_DENIED`
- **VERIFIED BY**: team-order `internal/handler` `TestGetReturnRequestStrangerDenied`

#### Scenario: Return read: Buyer, seller and admin keep access
- **WHEN** the return's buyer, the return's seller and an admin each call GetReturnRequest
- **THEN** each call returns OK
- **VERIFIED BY**: team-order `internal/handler` `TestGetReturnRequestPartiesAndAdminUnchanged`

#### Scenario: Return read: Anonymous caller is unauthenticated
- **WHEN** a caller without a principal calls GetReturnRequest
- **THEN** the call returns `UNAUTHENTICATED`
- **VERIFIED BY**: team-order `internal/handler` `TestGetReturnRequestAnonymousDenied`

#### Scenario: Return read: Unknown return
- **WHEN** the service principal calls GetReturnRequest with an id that does not exist
- **THEN** the call returns `NOT_FOUND`
- **VERIFIED BY**: team-order `internal/handler` `TestGetReturnRequestServiceUnknownNotFound`

#### Scenario: Return read: The service principal cannot change a return
- **WHEN** `service-team-engagement` with scope `order.read` calls UpdateReturnStatus to APPROVED
- **THEN** the call returns `PERMISSION_DENIED` and the return status is unchanged
- **VERIFIED BY**: team-order `internal/handler` `TestUpdateReturnStatusServicePrincipalDenied`
