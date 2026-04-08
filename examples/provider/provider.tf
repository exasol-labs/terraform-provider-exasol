provider "exasol" {
  host     = "exasol.example.com"
  port     = 8563
  user     = "sys"
  password = "exasol"

  # Set to false for self-signed certificates (e.g. Docker containers)
  validate_server_certificate = true
}

# PAT token authentication (detected automatically by exa_pat_ prefix)
provider "exasol" {
  alias    = "pat"
  host     = "exasol.example.com"
  user     = "my_user"
  password = "exa_pat_abc123..."
}
