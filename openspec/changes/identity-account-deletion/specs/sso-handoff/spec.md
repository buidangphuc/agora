## MODIFIED Requirements

### Requirement: The gateway forwards the session id only where it is needed

team-gateway SHALL set the `x-principal-sid` metadata on an upstream call only for the `platform.identity.v1.AuthService` procedures `BecomeSeller`, `CreateHandoffCode`, `PeekHandoffCode` and `RedeemHandoffCode` and the `platform.identity.v1.PrivacyService` procedure `RequestAccountDeletion`, and only when the verified token carries a `sid`. It SHALL NOT forward `x-principal-sid` on any other procedure or upstream service (`Register`, `Login`, `RefreshSession`, `CancelAccountDeletion`, `GetAccountDeletionStatus` and every non-identity service included).

#### Scenario: Session id is forwarded to BecomeSeller and CreateHandoffCode
- **WHEN** a user principal whose token has `sid=S` calls `BecomeSeller` and `CreateHandoffCode` through the edge
- **THEN** the outgoing metadata of each call contains `x-principal-sid=S`
- **VERIFIED BY**: team-gateway `internal/edge` `TestOutgoingForwardsSidOnlyToAllowlist`

#### Scenario: Session id is forwarded to RequestAccountDeletion
- **WHEN** a user principal whose token has `sid=S` calls `RequestAccountDeletion`, `CancelAccountDeletion` and `GetAccountDeletionStatus` through the edge
- **THEN** the outgoing metadata of `RequestAccountDeletion` contains `x-principal-sid=S`, and the outgoing metadata of `CancelAccountDeletion` and `GetAccountDeletionStatus` has no `x-principal-sid` key
- **VERIFIED BY**: team-gateway `internal/edge` `TestIdentityAccountDeletionRequestForwardsSessionID`

#### Scenario: Session id is never forwarded elsewhere
- **WHEN** the same principal calls `Login`, `RefreshSession`, `PeekHandoffCode` anonymously, or any procedure of another service (for example listing or order)
- **THEN** the outgoing metadata of each call has no `x-principal-sid` key and `x-principal-id`, `x-principal-type` and scopes are unchanged
- **VERIFIED BY**: team-gateway `internal/edge` `TestOutgoingOmitsSidOutsideAllowlist`

#### Scenario: A client-supplied sid header is dropped
- **WHEN** an anonymous caller sends its own `x-principal-sid` header on `RedeemHandoffCode` or on a non-allow-listed procedure, or a signed-in caller whose token has no `sid` sends it on `RequestAccountDeletion`
- **THEN** the upstream receives no `x-principal-sid`, because the value comes only from the verified token
- **VERIFIED BY**: team-gateway `internal/edge` `TestOutgoingOmitsSidOutsideAllowlist`
