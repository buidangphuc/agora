"""platform-recsys — offline PySpark ALS training job.

A new bounded context (AGENTS.md §6(c)): reads only the behavioral warehouse
produced by team-analytics and writes only its own artifact stores (its Qdrant
collections + its Redis keyspace). No request-serving surface — it is run by a
scheduler (a platform-gitops CronJob), not behind the gateway.
"""

__version__ = "0.1.0"
