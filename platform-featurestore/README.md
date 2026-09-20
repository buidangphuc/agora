# platform-featurestore

Centralized Feature Store for Agora ML & RecSys workloads.

## Features
- **Online Store**: Low-latency feature retrieval via Redis / In-memory fallback.
- **Offline Store**: Historical point-in-time joins for dataset generation.
- **Parity Validation**: Skew detection between online and offline feature stores.

## Commands
```bash
make test
```
