@buyer @shop @needsSeller @needsBuyer
Feature: Follow a seller shop
  As a logged-in buyer
  I want to follow and unfollow a seller's shop
  So that I can keep track of the shops I like under my account

  Background:
    Given I am logged in as a buyer via API

  Scenario: Buyer follows a shop and sees it under following
    When the buyer opens the seeded seller's shop page
    Then the shop shows a follow action
    When the buyer follows the shop
    Then the shop shows as followed
    And the followed shop appears under the buyer's following list

  Scenario: Buyer can unfollow a followed shop
    When the buyer opens the seeded seller's shop page
    And the buyer follows the shop
    And the buyer unfollows the shop
    Then the shop shows a follow action
