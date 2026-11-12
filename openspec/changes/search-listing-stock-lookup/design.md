## Clarifications

- 2026-11-12. Question: must the gateway answer HTTP 404 for the `GetListingStock` path? Answer: accept 404 OR 501 unimplemented. The requirement is that the gateway does not forward the call to team-search. team-gateway mounts the generated Connect handler with an embedded Unimplemented server, so the exact path answers 501. The trailing-slash path may answer 404.
