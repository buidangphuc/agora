## ADDED Requirements

### Requirement: Vague outcomes

The system SHALL respond to a buyer.

#### Scenario: Vague then

- **WHEN** a buyer submits a form
- **THEN** the system should respond properly and quickly

#### Scenario: Two behaviours

- **WHEN** a buyer submits an invalid form
- **THEN** the system SHALL return 422 and SHALL write an audit row, forbidden for other users
