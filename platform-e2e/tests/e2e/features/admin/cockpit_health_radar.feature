@admin
Feature: Admin Cockpit Health Radar
  As a system administrator
  I want to monitor microservices throughput and latency on the Admin Cockpit
  So that I can verify cluster health and observability signals

  Scenario: Admin inspects live telemetry on Cockpit
    When the admin opens the cockpit HUD
    Then the telemetry summary cards display throughput and latency metrics
