## Purpose

Defines who may read a seller's analytics (funnel, revenue breakdown, demand forecast)
from `team-analytics`.

## ADDED Requirements

### Requirement: A seller can read only their own analytics

`GetSellerFunnel`, `GetRevenueBreakdown` and `GetDemandForecast` SHALL require an
authenticated principal and SHALL serve a request only when the principal is a user whose
id equals the requested `seller_id`, or carries the `admin` scope. An anonymous caller
SHALL get `Unauthenticated`; any other caller SHALL get `PermissionDenied` with no data.
The check SHALL be enforced in `team-analytics` (the data owner) using the
gateway-forwarded principal; the gateway stays a plain forwarder.

#### Scenario: A seller reads their own funnel

- **WHEN** a signed-in seller requests `GetSellerFunnel` for their own id
- **THEN** the funnel is returned

#### Scenario: A seller cannot read another seller's revenue

- **WHEN** seller A requests `GetRevenueBreakdown` with seller B's id through the gateway
- **THEN** the gateway returns `permission_denied` and no revenue figures

#### Scenario: An anonymous caller is rejected

- **WHEN** `GetDemandForecast` is called through the gateway with no token
- **THEN** the gateway returns `unauthenticated`

#### Scenario: An admin can read any seller

- **WHEN** a principal with the `admin` scope requests `GetSellerFunnel` for any seller
- **THEN** the funnel is returned
