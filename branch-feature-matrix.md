# Branch Feature Matrix

| Branch | Classification | Decision | Reason |
|---|---|---|---|
| `main` | Baseline product | Include | Domain and migration reference. |
| `activity-report` | Business reporting | Include MVP | Activity rollups are useful and cheap. |
| `customer-report` | Business reporting | Include MVP | Customer rollups are central to billing. |
| `api-pagination` | API feature | Include MVP | Prevents large responses and supports integrations. |
| `download-invoice` | API feature | Include MVP | Useful integration endpoint with low cost. |
| `future-times` | Business rule | Include MVP | Implement `allow`, `deny`, `end_of_day`, `end_of_week`. |
| `invoice-meta-api` | API/admin integration | Include MVP | Metadata and invoice hooks support integrations. |
| `knvoice-meta-api` | Mixed duplicate/stale | Partial | Keep invoice voter/meta idea; ignore script churn. |
| `webhooks` | Integration feature | Include lightweight | Clean Go-native signed JSON delivery. |
| `dashboard` | Product + framework churn | Partial | Keep lightweight dashboard cards only. |
| `docker-ips` | Deployment | Include docs | Trusted proxy guidance for Docker/systemd. |
| `dev` | Framework/refactor | Mostly ignore | Extract invoice customer filter and audit idea only. |
| `release-2.56` | Release branch | Ignore | No rewrite product value. |
| `1.x` | Legacy maintenance | Migration reference | Old table shapes only. |

