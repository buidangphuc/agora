@admin
Feature: The buyer storefront has no admin area (team-frontend-admin-app)
  The admin app runs on its own host (ADR-0006). The storefront serves no /admin route:
  the standard not-found page, no redirect to the admin host.

  Scenario: Admin isolation: /admin on the buyer app is not found
    When the buyer storefront is requested at "/admin" and "/admin/users" without following redirects
    Then both buyer storefront responses are HTTP 404
