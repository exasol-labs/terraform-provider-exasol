# Test Directory

Manual test configurations for the Exasol Terraform Provider. For automated tests, see `internal/resources/acceptance_test.go`.

## Prerequisites

1. Exasol Docker container running:
   ```bash
   docker ps | grep exasol
   # If not running:
   docker run -d -p 8563:8563 --name exasol exasol/docker-db:latest
   ```

2. Provider built and installed locally:
   ```bash
   make install-local
   ```

## Test Suites

| Suite | Focus |
|---|---|
| suite-1-role-grants | Admin option handling, state transitions, case sensitivity |
| suite-2-object-privileges | Privilege list ordering, multiple privileges |
| suite-3-system-privileges | System privileges with admin options |
| suite-4-connection-grants | Connection access grants |
| suite-5-real-world | Production-like 4-layer data pipeline |

### Running

```bash
cd suite-1-role-grants  # or any suite
terraform init
terraform apply -auto-approve
terraform plan    # Should show "No changes"
terraform destroy -auto-approve
```

Or run all suites: `./run-tests.sh`

## Known Issues

### Transaction Collision During Destroy (Mitigated)

Delete operations are serialized using a global mutex to prevent Exasol transaction collision errors (SQL error code 40001). This makes parallel deletes slower but reliable.
