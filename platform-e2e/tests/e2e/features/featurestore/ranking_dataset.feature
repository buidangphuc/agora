@analytics @batch @integration
Feature: Impressions become labelled ranking rows
  `python -m featurestore dataset` builds rank_training@v1: one row per item impression, labelled from the click
  and add-to-cart events that carry the same impression_id. It runs here as the real job image: the first three
  scenarios against the real analytics export (one batch of beacons posted through the gateway edge, one export
  cycle waited out), the last against synthetic inputs built inside the image. Needs the rebuilt
  platform-featurestore image. (recsys-gbdt-trainer / governed-datasets)

  Scenario: A clicked impression becomes a positive training row
    When a buyer is shown listings in one recommendation row and clicks one of them with the same impressionId, the export cycle completes, and the dataset job runs
    Then rank_training@v1 has a row for that buyer, impression and listing with label 1

  Scenario: An add-to-cart is a stronger positive than a click
    When the buyer adds another listing of that row to the cart with the same impressionId, the export cycle completes, and the dataset job runs
    Then rank_training@v1 has a row for that listing with label 2

  Scenario: An impression without a click is a negative row
    When a third listing of that row is never clicked
    Then rank_training@v1 has a row for it with label 0 and the row's position

  Scenario: Impressions after AS_OF are not in the ranking dataset
    When the events hold one impression before AS_OF and one after it, and the dataset job runs with that AS_OF
    Then rank_training@v1 has a row for the first and none for the second
