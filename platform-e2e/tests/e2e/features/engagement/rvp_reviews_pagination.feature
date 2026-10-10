@engagement
Feature: ListReviews paginates a listing's reviews through the gateway
  A listing with more than 100 reviews stays fully reachable page by page.

  Scenario: Pages are disjoint and newest first
    Given a listing with 105 reviews
    When a client requests pages of 10 following next_cursor from the start
    Then every review appears exactly once newest first and the 11th page holds the 5 oldest with an empty next_cursor

  Scenario: Page size is bounded
    Given a listing with 105 reviews
    When a client requests page_size 500 for that listing
    Then at most 100 reviews are returned and next_cursor is set

  Scenario: A malformed cursor is rejected
    Given a listing with 105 reviews
    When a client sends the cursors "abc" and "-5"
    Then each call fails with InvalidArgument
